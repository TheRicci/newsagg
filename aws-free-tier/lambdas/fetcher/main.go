package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
)

type Article struct {
	ID          string `dynamodbav:"id" json:"id"`
	FeedKey     string `dynamodbav:"feed_key" json:"-"`
	Topic       string `dynamodbav:"topic" json:"topic"`
	Source      string `dynamodbav:"source" json:"source"`
	Title       string `dynamodbav:"title" json:"title"`
	URL         string `dynamodbav:"url" json:"url"`
	Summary     string `dynamodbav:"summary" json:"summary"`
	PublishedAt string `dynamodbav:"published_at" json:"published_at"`
	FetchedAt   string `dynamodbav:"fetched_at" json:"fetched_at"`
}

type FeedSource struct {
	Topic string
	Name  string
	URL   string
}

type FetchResult struct {
	Feeds       int `json:"feeds"`
	NewArticles int `json:"new_articles"`
	Published   int `json:"published"`
}

var sources = []FeedSource{
	{Topic: "ufology", Name: "newsnation", URL: "https://newsnationnow.com/space/ufo/feed"},
	{Topic: "neuroscience", Name: "neurosciencenews", URL: "https://neurosciencenews.com/feed"},
	{Topic: "neuroscience", Name: "sciencedaily", URL: "https://www.sciencedaily.com/rss/mind_brain/neuroscience.xml"},
	{Topic: "finance", Name: "marketwatch", URL: "https://feeds.marketwatch.com/marketwatch/topstories"},
	//{Topic: "finance", Name: "yahoofinance", URL: "https://finance.yahoo.com/news/rssindex"},
}

var (
	initOnce         sync.Once
	initErr          error
	db               *dynamodb.Client
	queue            *sqs.Client
	articlesTable    string
	topicCountsTable string
	queueURL         string
)

func initClients(ctx context.Context) error {
	initOnce.Do(func() {
		articlesTable = os.Getenv("ARTICLES_TABLE")
		topicCountsTable = os.Getenv("TOPIC_COUNTS_TABLE")
		queueURL = os.Getenv("ARTICLES_QUEUE_URL")
		if articlesTable == "" || topicCountsTable == "" || queueURL == "" {
			initErr = errors.New("ARTICLES_TABLE, TOPIC_COUNTS_TABLE, and ARTICLES_QUEUE_URL are required")
			return
		}

		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			initErr = fmt.Errorf("load aws config: %w", err)
			return
		}
		db = dynamodb.NewFromConfig(cfg)
		queue = sqs.NewFromConfig(cfg)
	})
	return initErr
}

func articleID(rawURL string) string {
	normalized := strings.TrimSpace(rawURL)
	if parsed, err := url.Parse(normalized); err == nil {
		parsed.Fragment = ""
		normalized = parsed.String()
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:16])
}

func cleanText(s string) string {
	if s == "" {
		return ""
	}
	tokenizer := html.NewTokenizer(strings.NewReader(s))
	var result strings.Builder
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			return strings.Join(strings.Fields(strings.TrimSpace(result.String())), " ")
		case html.TextToken:
			result.Write(tokenizer.Text())
		}
	}
}

func articleItem(article Article) map[string]ddbtypes.AttributeValue {
	return map[string]ddbtypes.AttributeValue{
		"id":           &ddbtypes.AttributeValueMemberS{Value: article.ID},
		"feed_key":     &ddbtypes.AttributeValueMemberS{Value: article.FeedKey},
		"topic":        &ddbtypes.AttributeValueMemberS{Value: article.Topic},
		"source":       &ddbtypes.AttributeValueMemberS{Value: article.Source},
		"title":        &ddbtypes.AttributeValueMemberS{Value: article.Title},
		"url":          &ddbtypes.AttributeValueMemberS{Value: article.URL},
		"summary":      &ddbtypes.AttributeValueMemberS{Value: article.Summary},
		"published_at": &ddbtypes.AttributeValueMemberS{Value: article.PublishedAt},
		"fetched_at":   &ddbtypes.AttributeValueMemberS{Value: article.FetchedAt},
	}
}

func putArticle(ctx context.Context, article Article) (bool, error) {
	_, err := db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(articlesTable),
		Item:                articleItem(article),
		ConditionExpression: aws.String("attribute_not_exists(id)"),
	})
	if err != nil {
		var conditional *ddbtypes.ConditionalCheckFailedException
		if errors.As(err, &conditional) {
			return false, nil
		}
		return false, fmt.Errorf("put article: %w", err)
	}
	return true, nil
}

func incrementTopic(ctx context.Context, topic string) error {
	_, err := db.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(topicCountsTable),
		Key: map[string]ddbtypes.AttributeValue{
			"topic": &ddbtypes.AttributeValueMemberS{Value: topic},
		},
		UpdateExpression: aws.String("SET updated_at = :now ADD #count :one"),
		ExpressionAttributeNames: map[string]string{
			"#count": "count",
		},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":one": &ddbtypes.AttributeValueMemberN{Value: "1"},
			":now": &ddbtypes.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
		},
	})
	if err != nil {
		return fmt.Errorf("increment topic: %w", err)
	}
	return nil
}

func publishArticle(ctx context.Context, article Article) error {
	body, err := json.Marshal(article)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	_, err = queue.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(string(body)),
	})
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	return nil
}

func fetchSource(ctx context.Context, src FeedSource) (int, int) {
	parser := gofeed.NewParser()
	feed, err := parser.ParseURL(src.URL)
	if err != nil {
		log.Printf("fetch %s failed: %v", src.Name, err)
		return 0, 0
	}

	newCount := 0
	publishedCount := 0
	now := time.Now().UTC()

	for _, item := range feed.Items {
		if item.Link == "" || item.Title == "" {
			continue
		}

		published := now
		if item.PublishedParsed != nil {
			published = item.PublishedParsed.UTC()
		}

		article := Article{
			ID:          articleID(item.Link),
			FeedKey:     "ALL",
			Topic:       src.Topic,
			Source:      src.Name,
			Title:       item.Title,
			URL:         item.Link,
			Summary:     cleanText(item.Description),
			PublishedAt: published.Format(time.RFC3339),
			FetchedAt:   now.Format(time.RFC3339),
		}

		inserted, err := putArticle(ctx, article)
		if err != nil {
			log.Printf("insert %s failed: %v", article.URL, err)
			continue
		}
		if !inserted {
			continue
		}

		newCount++
		if err := incrementTopic(ctx, article.Topic); err != nil {
			log.Printf("topic counter failed for %s: %v", article.Topic, err)
		}
		if err := publishArticle(ctx, article); err != nil {
			log.Printf("publish %s failed: %v", article.URL, err)
			continue
		}
		publishedCount++
	}

	log.Printf("fetched %s (%s): %d new, %d queued", src.Name, src.Topic, newCount, publishedCount)
	return newCount, publishedCount
}

func handler(ctx context.Context) (FetchResult, error) {
	if err := initClients(ctx); err != nil {
		return FetchResult{}, err
	}

	result := FetchResult{Feeds: len(sources)}
	for _, src := range sources {
		newCount, publishedCount := fetchSource(ctx, src)
		result.NewArticles += newCount
		result.Published += publishedCount
	}
	return result, nil
}

func main() {
	lambda.Start(handler)
}
