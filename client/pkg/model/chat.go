package model

import (
	"context"
	"log"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

// NewChatModel 创建新的聊天模型
func NewChatModel(ctx context.Context, apiKey, baseURL, modelName string) model.ChatModel {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	if modelName == "" {
		modelName = "gpt-4"
	}

	cm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  apiKey,
		Model:   modelName,
		BaseURL: baseURL,
	})
	if err != nil {
		log.Fatal(err)
	}
	return cm
}
