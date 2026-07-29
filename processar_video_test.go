package main

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func exigirFfmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg indisponível")
	}
}

func gerarVideoDeTeste(t *testing.T, destino string) {
	t.Helper()
	cmd := exec.Command(
		"ffmpeg",
		"-f", "lavfi",
		"-i", "testsrc=duration=3:size=320x240:rate=1",
		"-pix_fmt", "yuv420p",
		"-y", destino,
	)
	if saida, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("não foi possível gerar o vídeo de teste: %v\n%s", err, saida)
	}
}

func TestProcessarVideoGeraZipComFrames(t *testing.T) {
	exigirFfmpeg(t)

	dir := t.TempDir()
	entrada := filepath.Join(dir, "entrada.mp4")
	gerarVideoDeTeste(t, entrada)

	saidaDir := filepath.Join(dir, "saida")
	if err := os.MkdirAll(saidaDir, 0o755); err != nil {
		t.Fatalf("erro ao criar diretório de saída: %v", err)
	}

	caminhoZip, err := processarVideo(entrada, "video-teste", saidaDir)
	if err != nil {
		t.Fatalf("processarVideo retornou erro: %v", err)
	}

	if caminhoZip != filepath.Join(saidaDir, "video-teste.zip") {
		t.Fatalf("caminho do zip inesperado: %s", caminhoZip)
	}

	leitor, err := zip.OpenReader(caminhoZip)
	if err != nil {
		t.Fatalf("zip inválido: %v", err)
	}
	defer leitor.Close()

	if len(leitor.File) == 0 {
		t.Fatalf("o zip não contém frames")
	}
	for _, arquivo := range leitor.File {
		if !strings.HasSuffix(arquivo.Name, ".png") {
			t.Fatalf("arquivo inesperado no zip: %s", arquivo.Name)
		}
	}
}

func TestProcessarVideoFalhaComEntradaInvalida(t *testing.T) {
	exigirFfmpeg(t)

	dir := t.TempDir()
	entrada := filepath.Join(dir, "corrompido.mp4")
	if err := os.WriteFile(entrada, []byte("isto nao e um video"), 0o644); err != nil {
		t.Fatalf("erro ao escrever arquivo: %v", err)
	}

	if _, err := processarVideo(entrada, "video-ruim", dir); err == nil {
		t.Fatalf("esperava erro para entrada inválida, veio nil")
	}
}
