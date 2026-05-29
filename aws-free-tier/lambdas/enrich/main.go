package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

type Article struct {
	ID      string `json:"id"`
	Topic   string `json:"topic"`
	Source  string `json:"source"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Summary string `json:"summary"`
}

type EnrichmentResult struct {
	Summary    string   `json:"summary"`
	KeyPoints  []string `json:"key_points"`
	Context    string   `json:"context"`
	Tags       []string `json:"tags"`
	Confidence string   `json:"confidence"`
}

type BatchResult struct {
	Index      int              `json:"index"`
	Enrichment EnrichmentResult `json:"enrichment"`
}

type GeminiRequest struct {
	Contents         []GeminiContent  `json:"contents"`
	GenerationConfig GeminiGeneration `json:"generationConfig"`
}

type GeminiGeneration struct {
	ResponseMimeType string `json:"responseMimeType,omitempty"`
}

type GeminiContent struct {
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text string `json:"text"`
}

type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

var (
	initOnce      sync.Once
	initErr       error
	db            *dynamodb.Client
	ssmClient     *ssm.Client
	articlesTable string
	geminiKey     string
	geminiModel   string
	httpClient    = &http.Client{Timeout: 50 * time.Second}
)

func initClients(ctx context.Context) error {
	initOnce.Do(func() {
		articlesTable = os.Getenv("ARTICLES_TABLE")
		geminiModel = os.Getenv("GEMINI_MODEL")
		if geminiModel == "" {
			geminiModel = "gemini-2.5-flash-lite"
		}
		if articlesTable == "" {
			initErr = errors.New("ARTICLES_TABLE is required")
			return
		}

		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			initErr = fmt.Errorf("load aws config: %w", err)
			return
		}
		db = dynamodb.NewFromConfig(cfg)
		ssmClient = ssm.NewFromConfig(cfg)

		geminiKey = os.Getenv("GEMINI_API_KEY")
		if geminiKey != "" {
			return
		}

		parameterName := os.Getenv("GEMINI_API_KEY_PARAMETER")
		if parameterName == "" {
			initErr = errors.New("GEMINI_API_KEY or GEMINI_API_KEY_PARAMETER is required")
			return
		}

		out, err := ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
			Name:           aws.String(parameterName),
			WithDecryption: aws.Bool(true),
		})
		if err != nil {
			initErr = fmt.Errorf("read gemini api key parameter: %w", err)
			return
		}
		if out.Parameter == nil || out.Parameter.Value == nil || *out.Parameter.Value == "" {
			initErr = errors.New("gemini api key parameter is empty")
			return
		}
		geminiKey = *out.Parameter.Value
	})
	return initErr
}

func stripJSONFence(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "```") {
		return text
	}
	lines := strings.Split(text, "\n")
	if len(lines) <= 2 {
		return text
	}
	if strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		lines = lines[1 : len(lines)-1]
	} else {
		lines = lines[1:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func enrichBatch(ctx context.Context, articles []Article) (map[int]EnrichmentResult, error) {
	var prompt bytes.Buffer
	prompt.WriteString("You are a news enrichment assistant.\n")
	prompt.WriteString("For each article below, fetch the URL and read the full content if accessible.\n")
	prompt.WriteString("Return as much useful enrichment as you can find.\n\n")
	prompt.WriteString("Return ONLY a JSON array, no markdown, no explanation.\n")
	prompt.WriteString("Format:\n")
	prompt.WriteString(`[{"index":0,"enrichment":{"summary":"one sentence","key_points":["...","..."],"context":"background paragraph or empty string","tags":["tag1","tag2"],"confidence":"high|medium|low"}}]`)
	prompt.WriteString("\n\n")
	prompt.WriteString("confidence levels:\n")
	prompt.WriteString("- high: accessed full article\n")
	prompt.WriteString("- medium: accessed partial content\n")
	prompt.WriteString("- low: working only from title and description\n\n")

	for i, article := range articles {
		fmt.Fprintf(&prompt, "Article %d:\nTitle: %s\nDescription: %s\nURL: %s\n\n",
			i, article.Title, article.Summary, article.URL)
	}

	reqBody := GeminiRequest{
		Contents: []GeminiContent{
			{Parts: []GeminiPart{{Text: prompt.String()}}},
		},
		GenerationConfig: GeminiGeneration{ResponseMimeType: "application/json"},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini request: %w", err)
	}

	endpoint := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		url.PathEscape(geminiModel),
		url.QueryEscape(geminiKey),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build gemini request: %w", err)
	}
	req.Header.Set("content-type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return nil, fmt.Errorf("gemini error %d: %s", resp.StatusCode, string(b))
	}

	var geminiResp GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, fmt.Errorf("decode gemini response: %w", err)
	}
	if len(geminiResp.Candidates) == 0 ||
		len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, errors.New("gemini returned no content")
	}

	text := stripJSONFence(geminiResp.Candidates[0].Content.Parts[0].Text)
	var results []BatchResult
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		return nil, fmt.Errorf("parse gemini enrichments: %w raw=%s", err, text)
	}

	enrichments := make(map[int]EnrichmentResult, len(results))
	for _, result := range results {
		enrichments[result.Index] = result.Enrichment
	}
	return enrichments, nil
}

func saveEnrichment(ctx context.Context, article Article, enrichment EnrichmentResult) error {
	_, err := db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(articlesTable),
		Key: map[string]ddbtypes.AttributeValue{
			"id": &ddbtypes.AttributeValueMemberS{Value: article.ID},
		},
		UpdateExpression: aws.String("SET ai_summary = :summary, ai_key_points = :points, ai_context = :context, ai_tags = :tags, ai_confidence = :confidence, ai_enriched_at = :enriched_at"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":summary":     &ddbtypes.AttributeValueMemberS{Value: enrichment.Summary},
			":points":      stringList(enrichment.KeyPoints),
			":context":     &ddbtypes.AttributeValueMemberS{Value: enrichment.Context},
			":tags":        stringList(enrichment.Tags),
			":confidence":  &ddbtypes.AttributeValueMemberS{Value: enrichment.Confidence},
			":enriched_at": &ddbtypes.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
		},
	})
	if err != nil {
		return fmt.Errorf("update article %s: %w", article.ID, err)
	}
	return nil
}

func stringList(values []string) ddbtypes.AttributeValue {
	if len(values) == 0 {
		return &ddbtypes.AttributeValueMemberL{Value: []ddbtypes.AttributeValue{}}
	}
	items := make([]ddbtypes.AttributeValue, 0, len(values))
	for _, value := range values {
		items = append(items, &ddbtypes.AttributeValueMemberS{Value: value})
	}
	return &ddbtypes.AttributeValueMemberL{Value: items}
}

func failAll(records []events.SQSMessage) events.SQSEventResponse {
	failures := make([]events.SQSBatchItemFailure, 0, len(records))
	for _, record := range records {
		failures = append(failures, events.SQSBatchItemFailure{ItemIdentifier: record.MessageId})
	}
	return events.SQSEventResponse{BatchItemFailures: failures}
}

func handler(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
	if len(event.Records) == 0 {
		return events.SQSEventResponse{}, nil
	}
	if err := initClients(ctx); err != nil {
		log.Printf("init failed: %v", err)
		return failAll(event.Records), nil
	}

	articles := make([]Article, 0, len(event.Records))
	recordIDs := make([]string, 0, len(event.Records))
	failures := make([]events.SQSBatchItemFailure, 0)

	for _, record := range event.Records {
		var article Article
		if err := json.Unmarshal([]byte(record.Body), &article); err != nil {
			log.Printf("invalid message %s: %v", record.MessageId, err)
			failures = append(failures, events.SQSBatchItemFailure{ItemIdentifier: record.MessageId})
			continue
		}
		articles = append(articles, article)
		recordIDs = append(recordIDs, record.MessageId)
	}
	if len(articles) == 0 {
		return events.SQSEventResponse{BatchItemFailures: failures}, nil
	}

	enrichments, err := enrichBatch(ctx, articles)
	if err != nil {
		log.Printf("enrich batch failed: %v", err)
		for _, id := range recordIDs {
			failures = append(failures, events.SQSBatchItemFailure{ItemIdentifier: id})
		}
		return events.SQSEventResponse{BatchItemFailures: failures}, nil
	}

	for i, article := range articles {
		enrichment, ok := enrichments[i]
		if !ok {
			log.Printf("gemini response missing index %d for article %s", i, article.ID)
			failures = append(failures, events.SQSBatchItemFailure{ItemIdentifier: recordIDs[i]})
			continue
		}
		if err := saveEnrichment(ctx, article, enrichment); err != nil {
			log.Printf("save enrichment failed: %v", err)
			failures = append(failures, events.SQSBatchItemFailure{ItemIdentifier: recordIDs[i]})
		}
	}

	return events.SQSEventResponse{BatchItemFailures: failures}, nil
}

func main() {
	lambda.Start(handler)
}
