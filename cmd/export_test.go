package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/major0/kbchat/config"
	"github.com/major0/kbchat/export"
	"github.com/major0/kbchat/keybase"
)

func TestParseExportArgs(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantErr        bool
		errSubstr      string
		wantParallel   int
		wantVerbose    bool
		wantSkipAttach bool
		wantContinuous bool
		wantInterval   time.Duration
		wantLogFile    string
		wantDestDir    string
		wantFilters    []string
	}{
		{
			name:         "defaults with no flags",
			args:         []string{},
			wantParallel: DefaultParallel,
			wantInterval: DefaultInterval,
		},
		{
			name:         "-P flag",
			args:         []string{"-P", "8"},
			wantParallel: 8,
			wantInterval: DefaultInterval,
		},
		{
			name:         "--verbose flag",
			args:         []string{"--verbose"},
			wantVerbose:  true,
			wantParallel: DefaultParallel,
			wantInterval: DefaultInterval,
		},
		{
			name:           "--skip-attachments flag",
			args:           []string{"--skip-attachments"},
			wantSkipAttach: true,
			wantParallel:   DefaultParallel,
			wantInterval:   DefaultInterval,
		},
		{
			name:           "--continuous flag",
			args:           []string{"--continuous"},
			wantContinuous: true,
			wantParallel:   DefaultParallel,
			wantInterval:   DefaultInterval,
		},
		{
			name:         "--interval with valid duration",
			args:         []string{"--interval", "10m"},
			wantParallel: DefaultParallel,
			wantInterval: 10 * time.Minute,
		},
		{
			name:      "--interval with invalid duration",
			args:      []string{"--interval", "notaduration"},
			wantErr:   true,
			errSubstr: "invalid --interval value",
		},
		{
			name:         "--log-file flag",
			args:         []string{"--log-file", "/var/log/kbchat.log"},
			wantLogFile:  "/var/log/kbchat.log",
			wantParallel: DefaultParallel,
			wantInterval: DefaultInterval,
		},
		{
			name:         "destdir from positional arg",
			args:         []string{"/tmp/export"},
			wantDestDir:  "/tmp/export",
			wantParallel: DefaultParallel,
			wantInterval: DefaultInterval,
		},
		{
			name:         "filters from remaining positional args",
			args:         []string{"/tmp/export", "Chat/alice,bob", "Team/eng"},
			wantDestDir:  "/tmp/export",
			wantFilters:  []string{"Chat/alice,bob", "Team/eng"},
			wantParallel: DefaultParallel,
			wantInterval: DefaultInterval,
		},
		{
			name:           "combined flags and positional args",
			args:           []string{"-P", "2", "--verbose", "--continuous", "--interval", "30s", "--log-file", "out.log", "/data/backup", "Team/ops"},
			wantParallel:   2,
			wantVerbose:    true,
			wantContinuous: true,
			wantInterval:   30 * time.Second,
			wantLogFile:    "out.log",
			wantDestDir:    "/data/backup",
			wantFilters:    []string{"Team/ops"},
		},
		{
			name:      "invalid --parallel value",
			args:      []string{"-P", "abc"},
			wantErr:   true,
			errSubstr: "invalid --parallel value",
		},
		{
			name:      "--parallel zero",
			args:      []string{"-P", "0"},
			wantErr:   true,
			errSubstr: "invalid --parallel value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseExportArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err, tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if opts.Parallel != tt.wantParallel {
				t.Errorf("Parallel = %d, want %d", opts.Parallel, tt.wantParallel)
			}
			if opts.Verbose != tt.wantVerbose {
				t.Errorf("Verbose = %v, want %v", opts.Verbose, tt.wantVerbose)
			}
			if opts.SkipAttachments != tt.wantSkipAttach {
				t.Errorf("SkipAttachments = %v, want %v", opts.SkipAttachments, tt.wantSkipAttach)
			}
			if opts.Continuous != tt.wantContinuous {
				t.Errorf("Continuous = %v, want %v", opts.Continuous, tt.wantContinuous)
			}
			if opts.Interval != tt.wantInterval {
				t.Errorf("Interval = %v, want %v", opts.Interval, tt.wantInterval)
			}
			if opts.LogFile != tt.wantLogFile {
				t.Errorf("LogFile = %q, want %q", opts.LogFile, tt.wantLogFile)
			}
			if opts.DestDir != tt.wantDestDir {
				t.Errorf("DestDir = %q, want %q", opts.DestDir, tt.wantDestDir)
			}
			if !sliceEqual(opts.Filters, tt.wantFilters) {
				t.Errorf("Filters = %v, want %v", opts.Filters, tt.wantFilters)
			}
		})
	}
}

// sliceEqual compares two string slices for equality.
// Both nil and empty slices are treated as equivalent.
func sliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRunExportContinuousMultipleCycles(t *testing.T) {
	destDir := t.TempDir()
	cfg := &config.Config{StorePath: destDir}

	// Mock keybase client factory: returns nil client (mock runFunc ignores it).
	mockNewClient := func() (*keybase.Client, error) {
		return nil, nil
	}

	var cycles int
	mockRun := func(cfg export.Config, listClient export.ListAPI, newClient export.ClientFactory) (export.Summary, error) {
		cycles++
		return export.Summary{}, nil
	}

	// Sleep mock: cancel after 3 cycles by returning an error.
	mockSleep := func(ctx context.Context, d time.Duration) error {
		if cycles >= 3 {
			return context.Canceled
		}
		return nil
	}

	err := runExport(
		[]string{"--continuous", "--interval", "1s"},
		cfg, "testuser", mockNewClient, mockSleep, mockRun,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cycles != 3 {
		t.Errorf("expected 3 cycles, got %d", cycles)
	}
}

func TestRunExportContinuousSignalCancellation(t *testing.T) {
	destDir := t.TempDir()
	cfg := &config.Config{StorePath: destDir}

	mockNewClient := func() (*keybase.Client, error) {
		return nil, nil
	}

	var cycles int
	mockRun := func(cfg export.Config, listClient export.ListAPI, newClient export.ClientFactory) (export.Summary, error) {
		cycles++
		return export.Summary{}, nil
	}

	// Sleep mock: simulate context cancellation during sleep on first cycle.
	mockSleep := func(ctx context.Context, d time.Duration) error {
		return context.Canceled
	}

	err := runExport(
		[]string{"--continuous", "--interval", "1s"},
		cfg, "testuser", mockNewClient, mockSleep, mockRun,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cycles != 1 {
		t.Errorf("expected 1 cycle before cancellation, got %d", cycles)
	}
}

func TestRunExportSingleShot(t *testing.T) {
	destDir := t.TempDir()
	cfg := &config.Config{StorePath: destDir}

	mockNewClient := func() (*keybase.Client, error) {
		return nil, nil
	}

	var called int
	mockRun := func(cfg export.Config, listClient export.ListAPI, newClient export.ClientFactory) (export.Summary, error) {
		called++
		return export.Summary{Messages: 5}, nil
	}

	err := runExport(
		[]string{},
		cfg, "testuser", mockNewClient, nil, mockRun,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called != 1 {
		t.Errorf("expected export.Run called once, got %d", called)
	}
}

// --- Integration tests for export flow (Task 10.1) ---

// mockExportListClient implements export.ListAPI for integration tests.
type mockExportListClient struct {
	convs []keybase.ConvSummary
}

func (m *mockExportListClient) ListConversations() ([]keybase.ConvSummary, error) {
	return m.convs, nil
}
func (m *mockExportListClient) Close() error { return nil }

// mockExportWorkerClient implements export.ClientAPI for integration tests.
type mockExportWorkerClient struct {
	msgs map[string][]keybase.MsgSummary
}

func (m *mockExportWorkerClient) ReadConversation(convID string, known func(int) bool) ([]keybase.MsgSummary, error) {
	msgs := m.msgs[convID]
	var result []keybase.MsgSummary
	for _, msg := range msgs {
		if known != nil && known(msg.ID) {
			break
		}
		result = append(result, msg)
	}
	return result, nil
}

func (m *mockExportWorkerClient) GetMessages(_ string, _ []int) ([]keybase.MsgSummary, error) {
	return nil, nil
}

func (m *mockExportWorkerClient) DownloadAttachment(_ keybase.ChatChannel, _ int, _ string) error {
	return nil
}

func (m *mockExportWorkerClient) Close() error { return nil }

func TestIntegrationExportDestdirFromConfig(t *testing.T) {
	destDir := t.TempDir()
	cfg := &config.Config{StorePath: destDir}

	convs := []keybase.ConvSummary{
		{ID: "dm1", Channel: keybase.ChatChannel{Name: "testuser,alice", MembersType: "impteamnative"}},
	}
	msgs := map[string][]keybase.MsgSummary{
		"dm1": {
			{ID: 2, SentAtMs: 2000, Content: keybase.MsgContent{Type: "text", Text: &keybase.TextContent{Body: "hi"}}},
			{ID: 1, SentAtMs: 1000, Content: keybase.MsgContent{Type: "text", Text: &keybase.TextContent{Body: "hello"}}},
		},
	}

	lc := &mockExportListClient{convs: convs}
	wc := &mockExportWorkerClient{msgs: msgs}

	mockRun := func(cfg export.Config, listClient export.ListAPI, newClient export.ClientFactory) (export.Summary, error) {
		return export.Run(cfg, lc, func() (export.ClientAPI, error) { return wc, nil })
	}

	err := runExport(
		[]string{}, // no destdir arg → uses config store_path
		cfg, "testuser",
		func() (*keybase.Client, error) { return nil, nil },
		nil, mockRun,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify messages were exported to config store_path
	msgPath := filepath.Join(destDir, "Chats", "alice", "messages", "1", "message.json")
	if _, err := os.Stat(msgPath); err != nil {
		t.Errorf("message 1 not exported: %v", err)
	}
	msgPath2 := filepath.Join(destDir, "Chats", "alice", "messages", "2", "message.json")
	if _, err := os.Stat(msgPath2); err != nil {
		t.Errorf("message 2 not exported: %v", err)
	}
}

func TestIntegrationExportDestdirOverride(t *testing.T) {
	configDir := t.TempDir()
	overrideDir := t.TempDir()
	cfg := &config.Config{StorePath: configDir}

	convs := []keybase.ConvSummary{
		{ID: "dm1", Channel: keybase.ChatChannel{Name: "testuser,bob", MembersType: "impteamnative"}},
	}
	msgs := map[string][]keybase.MsgSummary{
		"dm1": {
			{ID: 1, SentAtMs: 1000, Content: keybase.MsgContent{Type: "text", Text: &keybase.TextContent{Body: "hey"}}},
		},
	}

	lc := &mockExportListClient{convs: convs}
	wc := &mockExportWorkerClient{msgs: msgs}

	mockRun := func(cfg export.Config, listClient export.ListAPI, newClient export.ClientFactory) (export.Summary, error) {
		return export.Run(cfg, lc, func() (export.ClientAPI, error) { return wc, nil })
	}

	err := runExport(
		[]string{overrideDir}, // positional destdir overrides config
		cfg, "testuser",
		func() (*keybase.Client, error) { return nil, nil },
		nil, mockRun,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify messages exported to override dir, not config dir
	msgPath := filepath.Join(overrideDir, "Chats", "bob", "messages", "1", "message.json")
	if _, err := os.Stat(msgPath); err != nil {
		t.Errorf("message not exported to override dir: %v", err)
	}

	// Config dir should be empty (no export there)
	entries, _ := os.ReadDir(configDir)
	if len(entries) != 0 {
		t.Errorf("config dir should be empty, has %d entries", len(entries))
	}
}

func TestIntegrationExportWithFilters(t *testing.T) {
	destDir := t.TempDir()
	cfg := &config.Config{StorePath: destDir}

	convs := []keybase.ConvSummary{
		{ID: "dm1", Channel: keybase.ChatChannel{Name: "testuser,alice", MembersType: "impteamnative"}},
		{ID: "dm2", Channel: keybase.ChatChannel{Name: "testuser,bob", MembersType: "impteamnative"}},
		{ID: "team1", Channel: keybase.ChatChannel{Name: "eng", MembersType: "team", TopicName: "general"}},
	}
	msgs := map[string][]keybase.MsgSummary{
		"dm1": {
			{ID: 1, SentAtMs: 1000, Content: keybase.MsgContent{Type: "text", Text: &keybase.TextContent{Body: "hi alice"}}},
		},
		"dm2": {
			{ID: 1, SentAtMs: 1000, Content: keybase.MsgContent{Type: "text", Text: &keybase.TextContent{Body: "hi bob"}}},
		},
		"team1": {
			{ID: 1, SentAtMs: 1000, Content: keybase.MsgContent{Type: "text", Text: &keybase.TextContent{Body: "team msg"}}},
		},
	}

	lc := &mockExportListClient{convs: convs}
	wc := &mockExportWorkerClient{msgs: msgs}

	mockRun := func(cfg export.Config, listClient export.ListAPI, newClient export.ClientFactory) (export.Summary, error) {
		return export.Run(cfg, lc, func() (export.ClientAPI, error) { return wc, nil })
	}

	// Filter to only Chat/alice
	err := runExport(
		[]string{destDir, "Chat/alice"},
		cfg, "testuser",
		func() (*keybase.Client, error) { return nil, nil },
		nil, mockRun,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Alice's messages should exist
	alicePath := filepath.Join(destDir, "Chats", "alice", "messages", "1", "message.json")
	if _, err := os.Stat(alicePath); err != nil {
		t.Errorf("alice message not exported: %v", err)
	}

	// Bob's messages should NOT exist (filtered out)
	bobPath := filepath.Join(destDir, "Chats", "bob", "messages", "1", "message.json")
	if _, err := os.Stat(bobPath); !os.IsNotExist(err) {
		t.Errorf("bob message should not be exported, got err: %v", err)
	}

	// Team messages should NOT exist (filtered out)
	teamPath := filepath.Join(destDir, "Teams", "eng", "general", "messages", "1", "message.json")
	if _, err := os.Stat(teamPath); !os.IsNotExist(err) {
		t.Errorf("team message should not be exported, got err: %v", err)
	}
}
