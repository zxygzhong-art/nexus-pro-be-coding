package service

import (
	"fmt"
	"sort"
	"strings"
)

func (c *Service) answerAgentPrompt(ctx RequestContext, prompt string) (string, []Reference, error) {
	account, _, err := c.resolveAccount(ctx)
	if err != nil {
		return "", nil, err
	}
	articles, err := c.store.ListKnowledgeArticles(goContext(ctx), ctx.TenantID)
	if err != nil {
		return "", nil, err
	}
	if len(articles) == 0 {
		return "当前租户没有可检索的知识库内容，已为你创建占位 Agent Run。", nil, nil
	}

	tokens := tokenize(prompt)
	matches := make([]KnowledgeArticle, 0)
	for _, article := range articles {
		decision, err := c.evaluateAuthz(ctx, account, CheckRequest{
			ApplicationCode: AppAgent,
			ResourceType:    ResourceKnowledgeArticle,
			ResourceID:      article.ID,
			Action:          ActionRead,
		})
		if err != nil {
			return "", nil, err
		}
		if !decision.Allowed {
			continue
		}
		if articleMatches(article, tokens) {
			matches = append(matches, article)
		}
	}
	if len(matches) == 0 {
		return "未检索到与当前问题匹配的知识库内容。", []Reference{}, nil
	}
	sortKnowledgeMatches(matches, tokens)

	refs := make([]Reference, 0, len(matches))
	lines := make([]string, 0, len(matches)+1)
	lines = append(lines, "基于租户知识库，给出以下建议：")
	for _, article := range matches {
		snippet := truncateRunes(article.Content, 120)
		refs = append(refs, Reference{
			Title:   article.Title,
			Snippet: snippet,
			Source:  "knowledge_article",
		})
		lines = append(lines, fmt.Sprintf("- %s: %s", article.Title, snippet))
	}
	if strings.Contains(strings.ToLower(prompt), "请假") || strings.Contains(strings.ToLower(prompt), "leave") {
		lines = append(lines, "建议优先引用请假制度、余额规则和审批流模板。")
	}
	return strings.Join(lines, "\n"), refs, nil
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}

func tokenize(value string) []string {
	value = strings.ToLower(value)
	fields := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ' ', '\n', '\t', '，', '。', ',', '.', ';', ':', '/', '\\', '|', '(', ')', '[', ']', '{', '}', '!', '?', '"', '\'', '、':
			return true
		default:
			return false
		}
	})
	return uniqueStrings(fields)
}

func articleMatches(article KnowledgeArticle, tokens []string) bool {
	return articleMatchScore(article, tokens) > 0
}

func sortKnowledgeMatches(items []KnowledgeArticle, tokens []string) {
	sort.SliceStable(items, func(i, j int) bool {
		left := articleMatchScore(items[i], tokens)
		right := articleMatchScore(items[j], tokens)
		if left != right {
			return left > right
		}
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].Title < items[j].Title
	})
}

func articleMatchScore(article KnowledgeArticle, tokens []string) int {
	title := strings.ToLower(article.Title)
	body := strings.ToLower(article.Content + " " + strings.Join(article.Tags, " "))
	score := 0
	for _, token := range tokens {
		if token == "" {
			continue
		}
		if strings.Contains(title, token) {
			score += 3
		}
		if strings.Contains(body, token) {
			score++
		}
	}
	return score
}
