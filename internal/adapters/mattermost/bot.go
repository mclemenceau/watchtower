package mattermost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/mclemenceau/watchtower/internal/application"
	"github.com/mclemenceau/watchtower/internal/domain"
	"github.com/mclemenceau/watchtower/internal/intent"
	"github.com/mclemenceau/watchtower/internal/ports"
	"github.com/mclemenceau/watchtower/internal/state"
)

// BotConfig holds all parameters needed to connect and operate the Mattermost bot.
type BotConfig struct {
	// ServerURL is the base HTTP URL of the Mattermost server (e.g. "http://192.168.1.193:8065").
	ServerURL string
	// Token is the bot token generated from the Mattermost System Console.
	Token string
	// BotUserID is the user ID of the bot account (used to filter out the bot's own posts).
	BotUserID string
	// Keyword is the trigger prefix users must include to address the bot (e.g. "@watchtower").
	// Comparison is case-insensitive. Empty defaults to "@watchtower".
	Keyword string
	// ReconnectDelay is how long to wait before attempting a WebSocket reconnect. Defaults to 5s.
	ReconnectDelay time.Duration
}

// ChannelNotifier implements ports.Notifier for a specific Mattermost channel,
// posting messages via the REST API using the bot token.
type ChannelNotifier struct {
	serverURL  string
	token      string
	channelID  string
	httpClient *http.Client
}

// Compile-time interface check.
var _ ports.Notifier = (*ChannelNotifier)(nil)

// NewChannelNotifier creates a ChannelNotifier that posts to channelID.
func NewChannelNotifier(serverURL, token, channelID string) *ChannelNotifier {
	return &ChannelNotifier{
		serverURL:  strings.TrimRight(serverURL, "/"),
		token:      token,
		channelID:  channelID,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type mmCreatePost struct {
	ChannelID string `json:"channel_id"`
	Message   string `json:"message"`
	RootID    string `json:"root_id,omitempty"` // non-empty → post inside a thread
}

// mmCreatedPost is the subset of the Mattermost POST response we care about.
type mmCreatedPost struct {
	ID string `json:"id"`
}

// Send posts text to the configured Mattermost channel via the REST API.
// It returns the ID of the created post so callers can track threads.
func (n *ChannelNotifier) Send(text string) error {
	_, err := n.post(text, "")
	return err
}

// post is the internal implementation shared by Send and ThreadNotifier.Send.
// rootID is empty for top-level posts and non-empty for thread replies.
// Returns the ID of the created post.
func (n *ChannelNotifier) post(text, rootID string) (string, error) {
	payload, err := json.Marshal(mmCreatePost{ChannelID: n.channelID, Message: text, RootID: rootID})
	if err != nil {
		return "", fmt.Errorf("ChannelNotifier: marshal: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, n.serverURL+"/api/v4/posts", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("ChannelNotifier: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+n.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ChannelNotifier: post: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return "", fmt.Errorf("ChannelNotifier: mattermost returned %d: %s", resp.StatusCode, body)
	}

	var created mmCreatedPost
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		// Non-fatal: we got a success status, we just can't track the post ID.
		return "", nil
	}
	return created.ID, nil
}

// ThreadNotifier implements ports.Notifier and posts every message as a reply
// inside an existing thread (root_id is fixed at construction time).
type ThreadNotifier struct {
	channel *ChannelNotifier
	rootID  string
	// onPost is called with each created post ID so the session can register it
	// as a known bot thread root. May be nil.
	onPost func(postID string)
}

// Compile-time interface check.
var _ ports.Notifier = (*ThreadNotifier)(nil)

// Send posts text as a reply inside the thread identified by rootID.
func (t *ThreadNotifier) Send(text string) error {
	postID, err := t.channel.post(text, t.rootID)
	if err != nil {
		return err
	}
	if postID != "" && t.onPost != nil {
		t.onPost(postID)
	}
	return nil
}

// BroadcastNotifier implements ports.Notifier by posting to every channel the
// bot has joined. It fetches the channel list fresh on each Send so new
// invitations are picked up without a restart.
type BroadcastNotifier struct {
	serverURL  string
	token      string
	botUserID  string
	httpClient *http.Client
}

// Compile-time interface check.
var _ ports.Notifier = (*BroadcastNotifier)(nil)

// NewBroadcastNotifier creates a BroadcastNotifier.
func NewBroadcastNotifier(serverURL, token, botUserID string) *BroadcastNotifier {
	return &BroadcastNotifier{
		serverURL:  strings.TrimRight(serverURL, "/"),
		token:      token,
		botUserID:  botUserID,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Send posts text to all channels the bot is currently a member of.
// Errors from individual channels are logged but do not abort the broadcast.
func (b *BroadcastNotifier) Send(text string) error {
	channels, err := b.fetchJoinedChannels()
	if err != nil {
		return fmt.Errorf("BroadcastNotifier: fetch channels: %w", err)
	}
	for _, ch := range channels {
		n := NewChannelNotifier(b.serverURL, b.token, ch)
		if err := n.Send(text); err != nil {
			log.Printf("BroadcastNotifier: channel %s: %v", ch, err)
		}
	}
	return nil
}

// fetchJoinedChannels returns the list of channel IDs the bot user belongs to.
func (b *BroadcastNotifier) fetchJoinedChannels() ([]string, error) {
	u := fmt.Sprintf("%s/api/v4/users/%s/channels", b.serverURL, b.botUserID)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("fetchJoinedChannels: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+b.token)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetchJoinedChannels: http: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("fetchJoinedChannels: mattermost returned %d: %s", resp.StatusCode, body)
	}

	var channels []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&channels); err != nil {
		return nil, fmt.Errorf("fetchJoinedChannels: decode: %w", err)
	}

	ids := make([]string, 0, len(channels))
	for _, ch := range channels {
		// Skip direct-message and group-message channels; broadcast only to O/P channels.
		if ch.Type != "D" && ch.Type != "G" {
			ids = append(ids, ch.ID)
		}
	}
	return ids, nil
}

// --- WebSocket event structs ---

// wsAuthChallenge is the authentication request sent to the server.
type wsAuthChallenge struct {
	Seq    int    `json:"seq"`
	Action string `json:"action"`
	Data   struct {
		Token string `json:"token"`
	} `json:"data"`
}

// wsEvent is a generic incoming WebSocket event from Mattermost.
type wsEvent struct {
	Event     string          `json:"event"`
	Data      json.RawMessage `json:"data"`
	Broadcast struct {
		ChannelID string `json:"channel_id"`
		UserID    string `json:"user_id"`
	} `json:"broadcast"`
}

// wsPostedData is the payload inside a "posted" event.
type wsPostedData struct {
	Post string `json:"post"` // JSON-encoded post object
}

// wsPost is the decoded post from a "posted" event.
type wsPost struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	UserID    string `json:"user_id"`
	Message   string `json:"message"`
	Type      string `json:"type"`    // "" = normal, "system_*" = system messages
	RootID    string `json:"root_id"` // non-empty when post is a thread reply
}

// RunBot connects to the Mattermost WebSocket API, authenticates as a bot,
// and dispatches every @-mention or thread-reply to application.Dispatch.
// It reconnects automatically on disconnect until ctx is cancelled.
//
// snap is used to read fresh artefact data on every dispatch. failures is used
// to read the current failure store; pass nil to disable failure commands.
// triggerAnalysis is called when the user requests on-demand analysis; pass nil to disable.
// resolver, logFetcher, llm, and launchpad are optional; pass nil to disable
// the respective features.
func RunBot(
	ctx context.Context,
	cfg BotConfig,
	snap *state.Snapshot,
	failures ports.FailureStorePort,
	defaultRelease string,
	summaryForProducts []string,
	summaryForReleases []string,
	httpClient *http.Client,
	resolver *intent.Resolver,
	logFetcher ports.LogFetcher,
	llm ports.LLMClient,
	launchpad ports.LaunchpadSource,
	triggerAnalysis func(release string) error,
) {
	if cfg.Token == "" || cfg.ServerURL == "" {
		log.Print("mattermost bot: disabled (MATTERMOST_SERVER_URL or MATTERMOST_BOT_TOKEN not set)")
		return
	}

	keyword := strings.ToLower(strings.TrimSpace(cfg.Keyword))
	if keyword == "" {
		keyword = "@watchtower"
	}

	reconnectDelay := cfg.ReconnectDelay
	if reconnectDelay <= 0 {
		reconnectDelay = 5 * time.Second
	}

	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	log.Printf("mattermost bot: connecting to %s (keyword: %q)", cfg.ServerURL, keyword)

	for {
		if err := runBotSession(ctx, cfg, keyword, snap, failures, defaultRelease, summaryForProducts, summaryForReleases, httpClient, resolver, logFetcher, llm, launchpad, triggerAnalysis); err != nil {
			if ctx.Err() != nil {
				log.Print("mattermost bot: context cancelled, shutting down")
				return
			}
			log.Printf("mattermost bot: session ended: %v — reconnecting in %s", err, reconnectDelay)
		}

		select {
		case <-ctx.Done():
			log.Print("mattermost bot: shutting down")
			return
		case <-time.After(reconnectDelay):
		}
	}
}

// runBotSession runs a single WebSocket session until it errors or ctx is cancelled.
func runBotSession(
	ctx context.Context,
	cfg BotConfig,
	keyword string,
	snap *state.Snapshot,
	failures ports.FailureStorePort,
	defaultRelease string,
	summaryForProducts []string,
	summaryForReleases []string,
	httpClient *http.Client,
	resolver *intent.Resolver,
	logFetcher ports.LogFetcher,
	llm ports.LLMClient,
	launchpad ports.LaunchpadSource,
	triggerAnalysis func(release string) error,
) error {
	wsURL, err := buildWSURL(cfg.ServerURL)
	if err != nil {
		return fmt.Errorf("build WS URL: %w", err)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		NetDial:          nil,
	}

	conn, _, err := dialer.DialContext(ctx, wsURL, http.Header{
		"Authorization": []string{"Bearer " + cfg.Token},
	})
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close() //nolint:errcheck

	log.Printf("mattermost bot: WebSocket connected to %s", wsURL)

	// Send authentication challenge.
	auth := wsAuthChallenge{Seq: 1, Action: "authentication_challenge"}
	auth.Data.Token = cfg.Token
	if err := conn.WriteJSON(auth); err != nil {
		return fmt.Errorf("send auth: %w", err)
	}

	// mu guards conn writes from concurrent dispatch goroutines.
	var mu sync.Mutex

	// botThreads tracks post IDs the bot has sent during this session.
	// A thread root ID present here means any reply in that thread should be
	// answered — no keyword required.
	var botThreads sync.Map // key: postID (string) → struct{}

	// registerPost adds a post ID (and its root, if it is itself a reply) to
	// the known-bot-thread set so future replies trigger a response.
	registerPost := func(postID, rootID string) {
		if postID != "" {
			botThreads.Store(postID, struct{}{})
		}
		// Also register the root so replies at any depth match.
		if rootID != "" {
			botThreads.Store(rootID, struct{}{})
		}
	}

	// cancelRead lets us stop the read loop when ctx is done.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Watchdog: close the connection when ctx is cancelled so ReadJSON unblocks.
	go func() {
		<-ctx.Done()
		mu.Lock()
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		mu.Unlock()
		conn.Close() //nolint:errcheck
	}()

	for {
		var ev wsEvent
		if err := conn.ReadJSON(&ev); err != nil {
			if ctx.Err() != nil {
				return nil // clean shutdown
			}
			return fmt.Errorf("read event: %w", err)
		}

		if ev.Event != "posted" {
			continue
		}

		var data wsPostedData
		if err := json.Unmarshal(ev.Data, &data); err != nil {
			log.Printf("mattermost bot: decode posted data: %v", err)
			continue
		}

		var post wsPost
		if err := json.Unmarshal([]byte(data.Post), &post); err != nil {
			log.Printf("mattermost bot: decode post: %v", err)
			continue
		}

		// Ignore system messages and our own posts.
		// When we see one of our own posts, register it so replies to it are tracked.
		if post.Type != "" {
			continue
		}
		if post.UserID == cfg.BotUserID {
			registerPost(post.ID, post.RootID)
			continue
		}

		lower := strings.ToLower(strings.TrimSpace(post.Message))

		// Determine why (if at all) we should respond.
		//
		//  1. Keyword mentioned anywhere in the message — always triggers.
		//     Mattermost @-mentions can appear mid-sentence: "hello @watchtower help".
		//  2. Reply inside a thread the bot is part of — triggers without keyword.
		//     We look up the reply's root ID in botThreads. Mattermost sets
		//     RootID on every reply; for the first reply to a top-level post,
		//     RootID equals that post's ID.
		isKeywordMention := strings.Contains(lower, keyword)
		_, inBotThread := botThreads.Load(post.RootID)
		isThreadReply := post.RootID != "" && inBotThread

		if !isKeywordMention && !isThreadReply {
			continue
		}

		// Extract the command text.
		// Take everything that follows the keyword in the message. This handles
		// both leading and mid-sentence mentions:
		//   "@watchtower help"        → "help"
		//   "hello @watchtower help"  → "help"
		//   "help @watchtower"        → "" → "help" (bare mention)
		// For thread replies with no keyword, use the raw message.
		var cmd string
		if isKeywordMention {
			idx := strings.Index(lower, keyword)
			cmd = strings.TrimSpace(post.Message[idx+len(keyword):])
		} else {
			cmd = post.Message
		}
		if cmd == "" {
			cmd = "greet"
		}

		artefacts, err := snap.Read()
		if err != nil {
			log.Printf("mattermost bot: read snapshot: %v", err)
			continue
		}

		var failureStore domain.FailureStore
		if failures != nil {
			if fs, ferr := failures.ReadFailures(); ferr == nil {
				failureStore = fs
			}
		}
		if failureStore == nil {
			failureStore = make(domain.FailureStore)
		}

		// Build the notifier. All replies go into a thread:
		// - If the triggering post is already inside a thread (RootID set),
		//   reply into that existing thread.
		// - If it is a new top-level message, reply into a thread rooted at
		//   that message (post.ID becomes the thread root). This creates a
		//   clean thread for every new @watchtower interaction.
		channelN := NewChannelNotifier(cfg.ServerURL, cfg.Token, post.ChannelID)
		var notifier ports.Notifier
		threadRoot := post.RootID
		if threadRoot == "" {
			// New top-level mention — root the thread at the user's post.
			threadRoot = post.ID
		}
		notifier = &ThreadNotifier{
			channel: channelN,
			rootID:  threadRoot,
			onPost: func(postID string) {
				registerPost(postID, threadRoot)
			},
		}

		// Session key: channelID+userID for multi-turn LLM clarification.
		sessionID := post.ChannelID + ":" + post.UserID

		go func(n ports.Notifier, sid, command string, fs domain.FailureStore) {
			if err := application.Dispatch(
				ctx, sid, command, artefacts, fs,
				defaultRelease, summaryForProducts, summaryForReleases,
				n, "", resolver, logFetcher, llm, launchpad,
				triggerAnalysis,
			); err != nil {
				log.Printf("mattermost bot: dispatch %q: %v", command, err)
			}
		}(notifier, sessionID, cmd, failureStore)
	}
}

// buildWSURL converts an HTTP(S) server URL to its WebSocket equivalent
// and appends the Mattermost WebSocket endpoint path.
func buildWSURL(serverURL string) (string, error) {
	u, err := url.Parse(strings.TrimRight(serverURL, "/"))
	if err != nil {
		return "", fmt.Errorf("parse server URL: %w", err)
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}
	u.Path = "/api/v4/websocket"
	return u.String(), nil
}
