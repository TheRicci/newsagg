package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type Article struct {
	ID           string   `dynamodbav:"id" json:"id"`
	FeedKey      string   `dynamodbav:"feed_key" json:"-"`
	Topic        string   `dynamodbav:"topic" json:"topic"`
	Source       string   `dynamodbav:"source" json:"source"`
	Title        string   `dynamodbav:"title" json:"title"`
	URL          string   `dynamodbav:"url" json:"url"`
	Summary      string   `dynamodbav:"summary" json:"summary"`
	PublishedAt  string   `dynamodbav:"published_at" json:"published_at"`
	FetchedAt    string   `dynamodbav:"fetched_at" json:"fetched_at"`
	AISummary    string   `dynamodbav:"ai_summary" json:"ai_summary,omitempty"`
	AIKeyPoints  []string `dynamodbav:"ai_key_points" json:"ai_key_points,omitempty"`
	AIContext    string   `dynamodbav:"ai_context" json:"ai_context,omitempty"`
	AITags       []string `dynamodbav:"ai_tags" json:"ai_tags,omitempty"`
	AIConfidence string   `dynamodbav:"ai_confidence" json:"ai_confidence,omitempty"`
	AIEnrichedAt string   `dynamodbav:"ai_enriched_at" json:"ai_enriched_at,omitempty"`
}

type TopicCount struct {
	Topic string `dynamodbav:"topic" json:"topic"`
	Count int64  `dynamodbav:"count" json:"count"`
}

var (
	initOnce         sync.Once
	initErr          error
	db               *dynamodb.Client
	articlesTable    string
	topicCountsTable string
	corsOrigin       string
)

func initClients(ctx context.Context) error {
	initOnce.Do(func() {
		articlesTable = os.Getenv("ARTICLES_TABLE")
		topicCountsTable = os.Getenv("TOPIC_COUNTS_TABLE")
		corsOrigin = firstCSV(os.Getenv("ACCESS_CONTROL_ORIGIN"))
		if corsOrigin == "" {
			corsOrigin = "*"
		}
		if articlesTable == "" || topicCountsTable == "" {
			initErr = errors.New("ARTICLES_TABLE and TOPIC_COUNTS_TABLE are required")
			return
		}

		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			initErr = fmt.Errorf("load aws config: %w", err)
			return
		}
		db = dynamodb.NewFromConfig(cfg)
	})
	return initErr
}

func firstCSV(value string) string {
	parts := strings.Split(value, ",")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func jsonResponse(status int, body any) (events.LambdaFunctionURLResponse, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return events.LambdaFunctionURLResponse{}, err
	}
	return events.LambdaFunctionURLResponse{
		StatusCode: status,
		Headers: map[string]string{
			"content-type":                 "application/json",
			"access-control-allow-origin":  corsOrigin,
			"access-control-allow-methods": "GET,OPTIONS",
			"access-control-allow-headers": "content-type",
		},
		Body: string(payload),
	}, nil
}

func textResponse(status int, body string) events.LambdaFunctionURLResponse {
	return events.LambdaFunctionURLResponse{
		StatusCode: status,
		Headers: map[string]string{
			"content-type":                 "text/plain; charset=utf-8",
			"access-control-allow-origin":  corsOrigin,
			"access-control-allow-methods": "GET,OPTIONS",
			"access-control-allow-headers": "content-type",
		},
		Body: body,
	}
}

func parsePositiveInt(value string, fallback, max int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	if parsed > max {
		return max
	}
	return parsed
}

func stringAttr(item map[string]ddbtypes.AttributeValue, key string) string {
	value, ok := item[key]
	if !ok {
		return ""
	}
	typed, ok := value.(*ddbtypes.AttributeValueMemberS)
	if !ok {
		return ""
	}
	return typed.Value
}

func int64Attr(item map[string]ddbtypes.AttributeValue, key string) int64 {
	value, ok := item[key]
	if !ok {
		return 0
	}
	typed, ok := value.(*ddbtypes.AttributeValueMemberN)
	if !ok {
		return 0
	}
	parsed, _ := strconv.ParseInt(typed.Value, 10, 64)
	return parsed
}

func stringListAttr(item map[string]ddbtypes.AttributeValue, key string) []string {
	value, ok := item[key]
	if !ok {
		return nil
	}
	typed, ok := value.(*ddbtypes.AttributeValueMemberL)
	if !ok {
		return nil
	}
	values := make([]string, 0, len(typed.Value))
	for _, entry := range typed.Value {
		if s, ok := entry.(*ddbtypes.AttributeValueMemberS); ok {
			values = append(values, s.Value)
		}
	}
	return values
}

func articleFromItem(item map[string]ddbtypes.AttributeValue) Article {
	return Article{
		ID:           stringAttr(item, "id"),
		FeedKey:      stringAttr(item, "feed_key"),
		Topic:        stringAttr(item, "topic"),
		Source:       stringAttr(item, "source"),
		Title:        stringAttr(item, "title"),
		URL:          stringAttr(item, "url"),
		Summary:      stringAttr(item, "summary"),
		PublishedAt:  stringAttr(item, "published_at"),
		FetchedAt:    stringAttr(item, "fetched_at"),
		AISummary:    stringAttr(item, "ai_summary"),
		AIKeyPoints:  stringListAttr(item, "ai_key_points"),
		AIContext:    stringAttr(item, "ai_context"),
		AITags:       stringListAttr(item, "ai_tags"),
		AIConfidence: stringAttr(item, "ai_confidence"),
		AIEnrichedAt: stringAttr(item, "ai_enriched_at"),
	}
}

func handleTopics(ctx context.Context) (events.LambdaFunctionURLResponse, error) {
	out, err := db.Scan(ctx, &dynamodb.ScanInput{
		TableName: aws.String(topicCountsTable),
	})
	if err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	topics := make([]TopicCount, 0, len(out.Items))
	for _, item := range out.Items {
		topics = append(topics, TopicCount{
			Topic: stringAttr(item, "topic"),
			Count: int64Attr(item, "count"),
		})
	}

	sort.Slice(topics, func(i, j int) bool {
		if topics[i].Count == topics[j].Count {
			return topics[i].Topic < topics[j].Topic
		}
		return topics[i].Count > topics[j].Count
	})

	return jsonResponse(http.StatusOK, topics)
}

func handleArticles(ctx context.Context, qs map[string]string) (events.LambdaFunctionURLResponse, error) {
	topic := qs["topic"]
	limit := parsePositiveInt(qs["limit"], 20, 100)
	page := parsePositiveInt(qs["page"], 1, 100)

	window := limit * page
	if window > 500 {
		window = 500
	}

	input := &dynamodb.QueryInput{
		TableName:        aws.String(articlesTable),
		ScanIndexForward: aws.Bool(false),
		Limit:            aws.Int32(int32(window)),
	}

	if topic == "" {
		input.IndexName = aws.String("feed-published-index")
		input.KeyConditionExpression = aws.String("feed_key = :feed")
		input.ExpressionAttributeValues = map[string]ddbtypes.AttributeValue{
			":feed": &ddbtypes.AttributeValueMemberS{Value: "ALL"},
		}
	} else {
		input.IndexName = aws.String("topic-published-index")
		input.KeyConditionExpression = aws.String("#topic = :topic")
		input.ExpressionAttributeNames = map[string]string{
			"#topic": "topic",
		}
		input.ExpressionAttributeValues = map[string]ddbtypes.AttributeValue{
			":topic": &ddbtypes.AttributeValueMemberS{Value: topic},
		}
	}

	out, err := db.Query(ctx, input)
	if err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	articles := make([]Article, 0, len(out.Items))
	for _, item := range out.Items {
		articles = append(articles, articleFromItem(item))
	}

	start := (page - 1) * limit
	if start >= len(articles) {
		articles = []Article{}
	} else {
		end := start + limit
		if end > len(articles) {
			end = len(articles)
		}
		articles = articles[start:end]
	}

	return jsonResponse(http.StatusOK, map[string]any{
		"page":     page,
		"limit":    limit,
		"articles": articles,
	})
}

func handleArticleByID(ctx context.Context, id string) (events.LambdaFunctionURLResponse, error) {
	out, err := db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(articlesTable),
		Key: map[string]ddbtypes.AttributeValue{
			"id": &ddbtypes.AttributeValueMemberS{Value: id},
		},
	})
	if err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if len(out.Item) == 0 {
		return jsonResponse(http.StatusNotFound, map[string]string{"error": "not found"})
	}

	article := articleFromItem(out.Item)

	return jsonResponse(http.StatusOK, article)
}

func handler(ctx context.Context, req events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	if err := initClients(ctx); err != nil {
		corsOrigin = "*"
		return jsonResponse(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	method := req.RequestContext.HTTP.Method
	if method == "" {
		method = req.Headers["x-http-method"]
	}
	if method == http.MethodOptions {
		return textResponse(http.StatusNoContent, ""), nil
	}
	if method != http.MethodGet {
		return jsonResponse(http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}

	path := strings.TrimRight(req.RawPath, "/")
	if path == "" {
		path = "/"
	}

	switch {
	case path == "/health":
		return textResponse(http.StatusOK, "api ok"), nil
	case path == "/topics":
		return handleTopics(ctx)
	case path == "/articles":
		return handleArticles(ctx, req.QueryStringParameters)
	case strings.HasPrefix(path, "/articles/"):
		id := strings.TrimPrefix(path, "/articles/")
		if id == "" {
			return jsonResponse(http.StatusBadRequest, map[string]string{"error": "missing id"})
		}
		return handleArticleByID(ctx, id)
	default:
		return jsonResponse(http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func main() {
	lambda.Start(handler)
}
