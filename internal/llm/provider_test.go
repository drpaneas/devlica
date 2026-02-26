package llm

import "testing"

func TestNewProvider_InvalidName(t *testing.T) {
	_, err := NewProvider(ProviderConfig{Name: "invalid", APIKey: "key", Model: "model"})
	if err == nil {
		t.Error("expected error for invalid provider name")
	}
}

func TestNewProvider_ValidNames(t *testing.T) {
	tests := []struct {
		name ProviderName
	}{
		{ProviderOpenAI},
		{ProviderAnthropic},
		{ProviderOllama},
	}
	for _, tt := range tests {
		t.Run(string(tt.name), func(t *testing.T) {
			p, err := NewProvider(ProviderConfig{
				Name:       tt.name,
				APIKey:     "fake-key",
				Model:      "model",
				OllamaHost: "http://localhost:11434",
			})
			if err != nil {
				t.Errorf("NewProvider(%q) unexpected error: %v", tt.name, err)
			}
			if p == nil {
				t.Errorf("NewProvider(%q) returned nil", tt.name)
			}
		})
	}
}
