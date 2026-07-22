package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestCriarZipEmpacotaOsArquivos(t *testing.T) {
	dir := t.TempDir()
	primeiro := filepath.Join(dir, "frame_0001.png")
	segundo := filepath.Join(dir, "frame_0002.png")
	if err := os.WriteFile(primeiro, []byte("aaa"), 0o644); err != nil {
		t.Fatalf("erro ao escrever arquivo: %v", err)
	}
	if err := os.WriteFile(segundo, []byte("bbbb"), 0o644); err != nil {
		t.Fatalf("erro ao escrever arquivo: %v", err)
	}

	destino := filepath.Join(dir, "saida.zip")
	if err := criarZip([]string{primeiro, segundo}, destino); err != nil {
		t.Fatalf("criarZip retornou erro: %v", err)
	}

	leitor, err := zip.OpenReader(destino)
	if err != nil {
		t.Fatalf("zip inválido: %v", err)
	}
	defer leitor.Close()

	nomes := map[string]bool{}
	for _, arquivo := range leitor.File {
		nomes[arquivo.Name] = true
	}
	if len(leitor.File) != 2 {
		t.Fatalf("esperava 2 arquivos no zip, veio %d", len(leitor.File))
	}
	if !nomes["frame_0001.png"] || !nomes["frame_0002.png"] {
		t.Fatalf("zip não contém os frames esperados: %v", nomes)
	}
}

func TestComPadrao(t *testing.T) {
	t.Setenv("FIAPX_TESTE_VAR", "definido")
	if valor := comPadrao("FIAPX_TESTE_VAR", "padrao"); valor != "definido" {
		t.Fatalf("esperava definido, veio %s", valor)
	}
	if valor := comPadrao("FIAPX_TESTE_INEXISTENTE", "padrao"); valor != "padrao" {
		t.Fatalf("esperava padrao, veio %s", valor)
	}
}
