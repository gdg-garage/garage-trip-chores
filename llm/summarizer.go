package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/gdg-garage/garage-trip-chores/storage"
)

type Summarizer struct {
	storage                 *storage.Storage
	discord                 *discordgo.Session
	logger                  *slog.Logger
	conf                    Config
	defaultDiscordChannelId string
	client                  Client
}

func NewSummarizer(
	storage *storage.Storage,
	discord *discordgo.Session,
	logger *slog.Logger,
	conf Config,
	defaultDiscordChannelId string,
) *Summarizer {
	return &Summarizer{
		storage:                 storage,
		discord:                 discord,
		logger:                  logger,
		conf:                    conf,
		defaultDiscordChannelId: defaultDiscordChannelId,
		client:                  NewGeminiClient(conf),
	}
}

// SetClient allows overriding the LLM client (useful for unit tests).
func (s *Summarizer) SetClient(c Client) {
	s.client = c
}

func (s *Summarizer) RunOnce(ctx context.Context) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("Panic recovered in LLM summarizer", "panic", r)
			retErr = fmt.Errorf("panic in LLM summarizer: %v", r)
		}
	}()

	if strings.TrimSpace(s.conf.ApiKey) == "" {
		s.logger.Debug("LLM API key is not configured, skipping chore summary")
		return nil
	}

	s.logger.Info("Starting LLM chore summary run")

	// Determine starting cutoff time from previous successful run
	var prevStats map[string]storage.AggregatedUserStats
	since := time.Now().Add(-12 * time.Hour) // fallback default

	lastLog, err := s.storage.GetLastSuccessfulLLMSummaryLog()
	if err != nil {
		s.logger.Error("Failed to query last LLM summary log", "error", err)
	} else if lastLog != nil {
		since = lastLog.RunAt
		if lastLog.StatsJSON != "" {
			var parsedStats map[string]storage.AggregatedUserStats
			if err := json.Unmarshal([]byte(lastLog.StatsJSON), &parsedStats); err == nil {
				prevStats = parsedStats
			}
		}
	}

	if prevStats == nil {
		prevStats = make(map[string]storage.AggregatedUserStats)
	}

	// 1. Gather tasks activity since previous run
	activity, err := s.storage.GetTasksActivitySince(since)
	if err != nil {
		s.logger.Error("Failed to fetch task activity for LLM summary", "error", err)
		return fmt.Errorf("failed to fetch task activity: %w", err)
	}

	if !activity.HasActivity() {
		s.logger.Info("No new tasks or chore activity since last run, skipping LLM summary", "since", since)
		return nil
	}

	// 2. Fetch current statistics
	currentStats, err := s.storage.GetAggregatedStats()
	if err != nil {
		s.logger.Error("Failed to fetch current aggregated stats for LLM summary", "error", err)
		return fmt.Errorf("failed to fetch stats: %w", err)
	}

	statsBytes, err := json.Marshal(currentStats)
	if err != nil {
		s.logger.Error("Failed to marshal current stats", "error", err)
		return fmt.Errorf("failed to marshal stats: %w", err)
	}
	statsJSON := string(statsBytes)

	// 3. Resolve user handles
	handles, err := s.storage.GetUserHandlesMap()
	if err != nil {
		s.logger.Warn("Failed to resolve user handles map", "error", err)
		handles = make(map[string]string)
	}

	// 4. Build LLM prompt
	now := time.Now()
	prompt, err := BuildPrompt(PromptData{
		StartTime:    since,
		EndTime:      now,
		Activity:     activity,
		CurrentStats: currentStats,
		PrevStats:    prevStats,
		UserHandles:  handles,
	})
	if err != nil {
		s.logger.Error("Failed to build LLM prompt", "error", err)
		return fmt.Errorf("failed to build prompt: %w", err)
	}

	// 5. Generate summary using LLM
	s.logger.Debug("Calling LLM API for summary")
	summary, err := s.client.GenerateContent(ctx, prompt)
	if err != nil {
		s.logger.Error("LLM API call failed", "error", err)
		_ = s.storage.SaveLLMSummaryLog(&storage.LLMSummaryLog{
			RunAt:      now,
			StatsJSON:  statsJSON,
			Summary:    fmt.Sprintf("Error: %v", err),
			TasksCount: activity.TotalActivityCount(),
			Status:     "error",
		})
		return fmt.Errorf("llm api call failed: %w", err)
	}

	summary = strings.TrimSpace(summary)
	s.logger.Info("LLM summary generated successfully", "length", len(summary))

	// 6. Send summary to Discord
	targetChannel := strings.TrimSpace(s.conf.DiscordChannelId)
	if targetChannel == "" {
		targetChannel = strings.TrimSpace(s.defaultDiscordChannelId)
	}

	if s.discord != nil && targetChannel != "" && targetChannel != "???" {
		s.sendToDiscord(targetChannel, summary)
	} else {
		s.logger.Warn("Discord session or channel ID not available, skipping Discord post", "channel", targetChannel)
	}

	// 7. Persist run log
	err = s.storage.SaveLLMSummaryLog(&storage.LLMSummaryLog{
		RunAt:      now,
		StatsJSON:  statsJSON,
		Summary:    summary,
		TasksCount: activity.TotalActivityCount(),
		Status:     "success",
	})
	if err != nil {
		s.logger.Error("Failed to save LLM summary log", "error", err)
	}

	return nil
}

func (s *Summarizer) sendToDiscord(channelId string, content string) {
	// Discord message limit is 2000 characters. If content exceeds it, chunk it safely.
	const maxLen = 1900
	if len(content) <= maxLen {
		_, err := s.discord.ChannelMessageSend(channelId, content)
		if err != nil {
			s.logger.Error("Failed to send LLM summary to Discord", "channelId", channelId, "error", err)
		}
		return
	}

	chunks := splitMessage(content, maxLen)
	for i, chunk := range chunks {
		_, err := s.discord.ChannelMessageSend(channelId, chunk)
		if err != nil {
			s.logger.Error("Failed to send chunk of LLM summary to Discord", "chunk", i, "channelId", channelId, "error", err)
		}
	}
}

func splitMessage(msg string, limit int) []string {
	var chunks []string
	lines := strings.Split(msg, "\n")
	var currentChunk strings.Builder

	for _, line := range lines {
		if currentChunk.Len()+len(line)+1 > limit {
			if currentChunk.Len() > 0 {
				chunks = append(chunks, currentChunk.String())
				currentChunk.Reset()
			}
		}
		if len(line) > limit {
			// Substring split for extremely long lines
			for len(line) > limit {
				chunks = append(chunks, line[:limit])
				line = line[limit:]
			}
		}
		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n")
		}
		currentChunk.WriteString(line)
	}
	if currentChunk.Len() > 0 {
		chunks = append(chunks, currentChunk.String())
	}
	return chunks
}
