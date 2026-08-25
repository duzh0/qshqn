package ai

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"qshqn/core/config"
	"qshqn/core/netx"
)

type ModelInfo struct {
	Name                       string   `json:"name"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
}

type ListModelsResponse struct {
	Models []ModelInfo `json:"models"`
}

func ListModels(ctx context.Context) ([]string, error) {
	APIkey := keysMan.GetKey()
	if APIkey == "" {
		return nil, fmt.Errorf("keys manager returned empty key")
	}

	url := fmt.Sprintf(config.Services.Gemini.ModelsEndpoint, APIkey)

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := netx.GetJSON[ListModelsResponse](ctxWithTimeout, url, nil)
	if err != nil {
		return nil, fmt.Errorf("gemini list models API error: %w", err)
	}

	var names []string
	for _, m := range resp.Models {
		if slices.Contains(m.SupportedGenerationMethods, "generateContent") {
			names = append(names, strings.TrimPrefix(m.Name, "models/"))
		}
	}

	slices.Sort(names)
	return names, nil
}
