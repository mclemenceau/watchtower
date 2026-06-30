package mattermost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mclemenceau/watchtower/internal/intent"
	"github.com/mclemenceau/watchtower/internal/state"
)

// PollerConfig holds the parameters needed to poll a Mattermost channel.
type PollerConfig struct {
	// ServerURL is the base URL of the Mattermost server (e.g. "https://chat.example.com").
	ServerURL string
	// Token is a Mattermost personal access token used for authentication.
	Token string
	// ChannelID is the ID of the channel to poll.
	ChannelID string
	// Interval is how often to poll for new posts.
	Interval time.Duration
	// Keyword is the trigger prefix that messages must start with (e.g. "@watchtower").
	// The comparison is case-insensitive. Posts not starting with this keyword are ignored.
	Keyword string
}

// mmPostList is the subset of the Mattermost POST list API response we care about.
type mmPostList struct {
	Order []string          `json:"order"`
	Posts map[string]mmPost `json:"posts"`
}

type mmPost struct {
	ID       string `json:"id"`
	Message  string `json:"message"`
	CreateAt int64  `json:"create_at"` // Unix milliseconds
	UserId   string `json:"user_id"`
}

// RunPoller starts a polling loop that reads new posts from a Mattermost channel,
// filters those that begin with cfg.Keyword, strips the keyword prefix, and
// dispatches the remaining text to Dispatch. It runs until ctx is cancelled.
//
// snap is used to read fresh artefact data on every dispatch. defaultRelease pins
// the status table to a release (empty = auto-detect).
//
// resolver is optional; pass nil to disable LLM-assisted intent resolution.
//
// The HTTP client used is the package-level default; inject a custom one via
// the httpClient parameter to override in tests.
func RunPoller(ctx context.Context, cfg PollerConfig, snap *state.Snapshot, defaultRelease string, hook WebhookClient, httpClient *http.Client, resolver *intent.Resolver) {
	if cfg.Token == "" || cfg.ChannelID == "" || cfg.ServerURL == "" {
		log.Print("mattermost poller: disabled (MATTERMOST_TOKEN, MATTERMOST_SERVER_URL, or MATTERMOST_CHANNEL_ID not set)")
		return
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	keyword := strings.ToLower(strings.TrimSpace(cfg.Keyword))
	if keyword == "" {
		keyword = "@watchtower"
	}

	interval := cfg.Interval
	if interval <= 0 {
		interval = 15 * time.Second
	}

	// seed: treat all posts already in the channel as seen so we only react to
	// messages sent after the bot starts.
	since := time.Now().UnixMilli()

	log.Printf("mattermost poller: watching channel %s every %s for keyword %q", cfg.ChannelID, interval, keyword)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Print("mattermost poller: shutting down")
			return
		case <-ticker.C:
			posts, newSince, err := fetchNewPosts(ctx, httpClient, cfg, since)
			if err != nil {
				log.Printf("mattermost poller: fetch error: %v", err)
				continue
			}
			since = newSince

			for _, post := range posts {
				lower := strings.ToLower(strings.TrimSpace(post.Message))
				if !strings.HasPrefix(lower, keyword) {
					continue // not addressed to us
				}
				// Strip the keyword and any leading whitespace to get the command.
				cmd := strings.TrimSpace(post.Message[len(keyword):])
				if cmd == "" {
					// bare keyword with no command — show help
					cmd = "help"
				}
				artefacts, err := snap.Read()
				if err != nil {
					log.Printf("mattermost poller: read snapshot: %v", err)
					continue
				}
				// Use channelID+userID as the session key for multi-turn clarification.
				sessionID := cfg.ChannelID + ":" + post.UserId
				if err := Dispatch(ctx, sessionID, cmd, artefacts, defaultRelease, hook, "", resolver); err != nil {
					log.Printf("mattermost poller: dispatch %q: %v", cmd, err)
				}
			}
		}
	}
}

// fetchNewPosts calls the Mattermost Posts API for posts created after sinceMs
// (Unix milliseconds). Returns the posts in chronological order and an updated
// sinceMs cursor (the maximum CreateAt seen, or the original value if no posts
// were returned).
func fetchNewPosts(ctx context.Context, client *http.Client, cfg PollerConfig, sinceMs int64) ([]mmPost, int64, error) {
	url := fmt.Sprintf("%s/api/v4/channels/%s/posts?since=%d", strings.TrimRight(cfg.ServerURL, "/"), cfg.ChannelID, sinceMs)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, sinceMs, fmt.Errorf("fetchNewPosts: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, sinceMs, fmt.Errorf("fetchNewPosts: http: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, sinceMs, fmt.Errorf("fetchNewPosts: mattermost returned %d: %s", resp.StatusCode, body)
	}

	var pl mmPostList
	if err := json.NewDecoder(resp.Body).Decode(&pl); err != nil {
		return nil, sinceMs, fmt.Errorf("fetchNewPosts: decode: %w", err)
	}

	// Build result in order (pl.Order is newest-first, so reverse it).
	posts := make([]mmPost, 0, len(pl.Order))
	newSince := sinceMs
	for _, id := range pl.Order {
		p, ok := pl.Posts[id]
		if !ok {
			continue
		}
		posts = append(posts, p)
		if p.CreateAt > newSince {
			newSince = p.CreateAt
		}
	}

	// Reverse so we process oldest first.
	for i, j := 0, len(posts)-1; i < j; i, j = i+1, j-1 {
		posts[i], posts[j] = posts[j], posts[i]
	}

	// Advance cursor by 1ms so the next call does not re-fetch the last post.
	if newSince > sinceMs {
		newSince++
	}

	return posts, newSince, nil
}
