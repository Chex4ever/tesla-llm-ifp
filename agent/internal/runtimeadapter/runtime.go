package runtimeadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type DesiredModel struct {
	ModelID        string `json:"model_id"`
	Format         string `json:"format"`
	OllamaTag      string `json:"ollama_tag"`
	MinioObject    string `json:"minio_object"`
	DownloadURL    string `json:"download_url"`
	ChecksumSHA256 string `json:"checksum_sha256"`
	MinVRAMMb      int    `json:"min_vram_mb"`
}

type Runtime interface {
	Health(ctx context.Context) (healthy bool, models []string, err error)
	EnsureModel(ctx context.Context, m DesiredModel) error
	Proxy(ctx context.Context, path string, body []byte) (status int, respBody []byte, err error)
}

type Ollama struct {
	Base string
	HTTP *http.Client
}

func NewOllama(base string) *Ollama {
	return &Ollama{Base: base, HTTP: &http.Client{Timeout: 10 * time.Minute}}
}

func (o *Ollama) Health(ctx context.Context) (bool, []string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.Base+"/api/tags", nil)
	if err != nil {
		return false, nil, err
	}
	resp, err := o.HTTP.Do(req)
	if err != nil {
		return false, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return false, nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var out struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return true, nil, nil
	}
	names := make([]string, 0, len(out.Models))
	for _, m := range out.Models {
		names = append(names, m.Name)
	}
	return true, names, nil
}

func (o *Ollama) EnsureModel(ctx context.Context, m DesiredModel) error {
	tag := m.OllamaTag
	if tag == "" {
		tag = m.ModelID
	}
	if m.DownloadURL != "" && m.Format == "gguf" {
		dir := filepath.Join(os.TempDir(), "pirate-models")
		_ = os.MkdirAll(dir, 0o755)
		dest := filepath.Join(dir, filepath.Base(m.MinioObject))
		if err := downloadFile(ctx, m.DownloadURL, dest); err != nil {
			return err
		}
		// Modelfile import
		modelfile := fmt.Sprintf("FROM %s\n", dest)
		mf := filepath.Join(dir, tag+".Modelfile")
		if err := os.WriteFile(mf, []byte(modelfile), 0o644); err != nil {
			return err
		}
		body, _ := json.Marshal(map[string]string{"name": tag, "modelfile": modelfile})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.Base+"/api/create", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := o.HTTP.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	body, _ := json.Marshal(map[string]string{"name": tag})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.Base+"/api/pull", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("pull status %d", resp.StatusCode)
	}
	return nil
}

func (o *Ollama) Proxy(ctx context.Context, path string, body []byte) (int, []byte, error) {
	url := o.Base + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, b, err
}

type VLLM struct {
	Base string
	HTTP *http.Client
}

func NewVLLM(base string) *VLLM {
	return &VLLM{Base: base, HTTP: &http.Client{Timeout: 10 * time.Minute}}
}

func (v *VLLM) Health(ctx context.Context) (bool, []string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.Base+"/v1/models", nil)
	if err != nil {
		return false, nil, err
	}
	resp, err := v.HTTP.Do(req)
	if err != nil {
		return false, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return false, nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	names := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		names = append(names, m.ID)
	}
	return true, names, nil
}

func (v *VLLM) EnsureModel(ctx context.Context, m DesiredModel) error {
	// vLLM models are typically preloaded via server launch flags; agent records desire only.
	return nil
}

func (v *VLLM) Proxy(ctx context.Context, path string, body []byte) (int, []byte, error) {
	url := v.Base + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := v.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, b, err
}

func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("download status %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}
