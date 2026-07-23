package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	filaProcessar = "videos.processar"
	filaResultado = "videos.resultado"
)

type mensagemProcessar struct {
	VideoID        string `json:"videoId"`
	CaminhoEntrada string `json:"caminhoEntrada"`
}

type mensagemResultado struct {
	VideoID      string `json:"videoId"`
	Status       string `json:"status"`
	CaminhoZip   string `json:"caminhoZip,omitempty"`
	MensagemErro string `json:"mensagemErro,omitempty"`
}

func main() {
	url := comPadrao("RABBITMQ_URL", "amqp://guest:guest@localhost:5672")
	saidaDir := comPadrao("OUTPUTS_DIR", "/data/outputs")
	metricasAddr := comPadrao("METRICS_ADDR", ":9091")

	if err := os.MkdirAll(saidaDir, 0o755); err != nil {
		log.Fatalf("não foi possível criar %s: %v", saidaDir, err)
	}

	iniciarServidorMetricas(metricasAddr)

	conexao := conectar(url)
	defer conexao.Close()

	canal, err := conexao.Channel()
	if err != nil {
		log.Fatalf("erro ao abrir canal: %v", err)
	}
	defer canal.Close()

	declararFilas(canal)

	if err := canal.Qos(1, 0, false); err != nil {
		log.Fatalf("erro ao configurar qos: %v", err)
	}

	entregas, err := canal.Consume(filaProcessar, "", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("erro ao consumir %s: %v", filaProcessar, err)
	}

	log.Printf("worker aguardando mensagens em %s", filaProcessar)
	for entrega := range entregas {
		tratar(canal, saidaDir, entrega)
	}
}

func tratar(canal *amqp.Channel, saidaDir string, entrega amqp.Delivery) {
	var msg mensagemProcessar
	if err := json.Unmarshal(entrega.Body, &msg); err != nil {
		log.Printf("mensagem inválida descartada: %v", err)
		_ = entrega.Ack(false)
		return
	}

	log.Printf("processando vídeo %s", msg.VideoID)
	publicarResultado(canal, mensagemResultado{VideoID: msg.VideoID, Status: "PROCESSANDO"})

	inicio := time.Now()
	caminhoZip, err := processarVideo(msg.CaminhoEntrada, msg.VideoID, saidaDir)
	duracaoProcessamento.Observe(time.Since(inicio).Seconds())
	if err != nil {
		log.Printf("falha ao processar %s: %v", msg.VideoID, err)
		videosProcessados.WithLabelValues("ERRO").Inc()
		publicarResultado(canal, mensagemResultado{
			VideoID:      msg.VideoID,
			Status:       "ERRO",
			MensagemErro: err.Error(),
		})
	} else {
		log.Printf("vídeo %s concluído: %s", msg.VideoID, caminhoZip)
		videosProcessados.WithLabelValues("CONCLUIDO").Inc()
		publicarResultado(canal, mensagemResultado{
			VideoID:    msg.VideoID,
			Status:     "CONCLUIDO",
			CaminhoZip: caminhoZip,
		})
	}

	_ = entrega.Ack(false)
}

func processarVideo(caminhoEntrada, videoID, saidaDir string) (string, error) {
	dirTemp, err := os.MkdirTemp("", "frames-"+videoID+"-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dirTemp)

	padrao := filepath.Join(dirTemp, "frame_%04d.png")
	cmd := exec.Command("ffmpeg", "-i", caminhoEntrada, "-vf", "fps=1", "-y", padrao)
	if saida, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ffmpeg: %s", string(saida))
	}

	frames, err := filepath.Glob(filepath.Join(dirTemp, "*.png"))
	if err != nil {
		return "", err
	}
	if len(frames) == 0 {
		return "", errors.New("nenhum frame extraído do vídeo")
	}

	caminhoZip := filepath.Join(saidaDir, videoID+".zip")
	if err := criarZip(frames, caminhoZip); err != nil {
		return "", err
	}
	return caminhoZip, nil
}

func criarZip(arquivos []string, caminhoZip string) error {
	saida, err := os.Create(caminhoZip)
	if err != nil {
		return err
	}
	defer saida.Close()

	escritor := zip.NewWriter(saida)
	defer escritor.Close()

	for _, arquivo := range arquivos {
		if err := adicionarAoZip(escritor, arquivo); err != nil {
			return err
		}
	}
	return nil
}

func adicionarAoZip(escritor *zip.Writer, caminho string) error {
	arquivo, err := os.Open(caminho)
	if err != nil {
		return err
	}
	defer arquivo.Close()

	info, err := arquivo.Stat()
	if err != nil {
		return err
	}

	cabecalho, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	cabecalho.Name = filepath.Base(caminho)
	cabecalho.Method = zip.Deflate

	destino, err := escritor.CreateHeader(cabecalho)
	if err != nil {
		return err
	}
	_, err = io.Copy(destino, arquivo)
	return err
}

func publicarResultado(canal *amqp.Channel, msg mensagemResultado) {
	corpo, err := json.Marshal(msg)
	if err != nil {
		log.Printf("erro ao serializar resultado: %v", err)
		return
	}

	ctx, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()

	err = canal.PublishWithContext(ctx, "", filaResultado, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         corpo,
	})
	if err != nil {
		log.Printf("erro ao publicar resultado: %v", err)
	}
}

func declararFilas(canal *amqp.Channel) {
	for _, fila := range []string{filaProcessar, filaResultado} {
		if _, err := canal.QueueDeclare(fila, true, false, false, false, nil); err != nil {
			log.Fatalf("erro ao declarar fila %s: %v", fila, err)
		}
	}
}

func conectar(url string) *amqp.Connection {
	for tentativa := 1; tentativa <= 10; tentativa++ {
		conexao, err := amqp.Dial(url)
		if err == nil {
			return conexao
		}
		log.Printf("RabbitMQ indisponível (tentativa %d): %v", tentativa, err)
		time.Sleep(3 * time.Second)
	}
	log.Fatalf("não foi possível conectar ao RabbitMQ em %s", url)
	return nil
}

func comPadrao(chave, padrao string) string {
	if valor := os.Getenv(chave); valor != "" {
		return valor
	}
	return padrao
}
