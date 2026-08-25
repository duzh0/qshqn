package ai

import (
	"time"

	"qshqn/core/config"
)

const (
	RoleUser  Role = "user"
	RoleModel Role = "model"

	TypeString  SchemaType = "STRING"
	TypeNumber  SchemaType = "NUMBER"
	TypeInteger SchemaType = "INTEGER"
	TypeBoolean SchemaType = "BOOLEAN"
	TypeArray   SchemaType = "ARRAY"
	TypeObject  SchemaType = "OBJECT"
)

type Role string

func (r Role) String() string { return string(r) }

type SchemaType string

type Schema struct {
	Type        SchemaType         `json:"type"`
	Description string             `json:"description,omitempty"`
	Nullable    bool               `json:"nullable,omitempty"`
	Enum        []string           `json:"enum,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty"`
	Required    []string           `json:"required,omitempty"`
	Items       *Schema            `json:"items,omitempty"`
}

type ApiRequest struct {
	Contents          []Content          `json:"contents"`
	SafetySettings    []SafetySetting    `json:"safetySettings,omitempty"`
	GenerationConfig  *GenerationConfig  `json:"generationConfig,omitempty"`
	SystemInstruction *SystemInstruction `json:"systemInstruction,omitempty"`
}

type Content struct {
	Role  Role   `json:"role"`
	Parts []Part `json:"parts"`
}

type Part struct {
	Text string `json:"text,omitempty"`
}

type SafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type GenerationConfig struct {
	Temperature      float64 `json:"temperature,omitempty"`
	MaxOutputTokens  int     `json:"maxOutputTokens,omitempty"`
	ResponseMimeType string  `json:"responseMimeType,omitempty"`
	ResponseSchema   *Schema `json:"response_schema,omitempty"`
}

type SystemInstruction struct {
	Parts []Part `json:"parts"`
}

type ApiResponse struct {
	Candidates     []Candidate     `json:"candidates"`
	UsageMetadata  *UsageMetadata  `json:"usageMetadata"`
	PromptFeedback *PromptFeedback `json:"promptFeedback,omitempty"`
}

type Candidate struct {
	Content      Content `json:"content"`
	FinishReason string  `json:"finishReason,omitempty"`
}

type PromptFeedback struct {
	BlockReason string `json:"blockReason"`
}

type UsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type GenerateTextOptions struct {
	Timeout           time.Duration
	Model             string
	Text              string
	GenConfig         *GenerationConfig
	SystemInstruction *SystemInstruction
	History           []Content
}

func (o *GenerateTextOptions) fillDefaults() {
	if o.Model == "" {
		o.Model = config.Services.Gemini.DefaultModel
	}

	if o.GenConfig == nil {
		o.GenConfig = &GenerationConfig{
			Temperature: 1,
		}
	}

	if o.Timeout == 0 {
		o.Timeout = 120 * time.Second
	}
}
