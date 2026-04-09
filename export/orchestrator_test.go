package export

import (
	"math/rand"
	"testing"
	"testing/quick"

	"github.com/major0/kbchat/keybase"
)

// Feature: keybase-go-export, Property 10: Export summary contains all counts.
func TestPropertySummaryCounts(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		n := r.Intn(10) + 1
		results := make([]Result, n)
		var wantMsgs, wantAtt, wantErr int
		for i := range results {
			m := r.Intn(100)
			a := r.Intn(20)
			e := r.Intn(5)
			results[i] = Result{
				MessagesExported:      m,
				AttachmentsDownloaded: a,
			}
			for range e {
				results[i].Errors = append(results[i].Errors, nil)
			}
			wantMsgs += m
			wantAtt += a
			wantErr += e
		}

		s := SummarizeResults(results)
		if s.Conversations != n {
			t.Logf("conversations: got %d, want %d", s.Conversations, n)
			return false
		}
		if s.Messages != wantMsgs {
			t.Logf("messages: got %d, want %d", s.Messages, wantMsgs)
			return false
		}
		if s.Attachments != wantAtt {
			t.Logf("attachments: got %d, want %d", s.Attachments, wantAtt)
			return false
		}
		if s.Errors != wantErr {
			t.Logf("errors: got %d, want %d", s.Errors, wantErr)
			return false
		}
		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Fatal(err)
	}
}

type mockListClient struct {
	convs []keybase.ConvSummary
	err   error
}

func (m *mockListClient) ListConversations() ([]keybase.ConvSummary, error) {
	return m.convs, m.err
}
func (m *mockListClient) Close() error { return nil }

func TestRun_EmptyConversationList(t *testing.T) {
	cfg := Config{DestDir: t.TempDir(), Parallel: 2, SelfUsername: "self"}
	lc := &mockListClient{convs: nil}
	s, err := Run(cfg, lc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Conversations != 0 {
		t.Errorf("conversations = %d, want 0", s.Conversations)
	}
}

func TestRun_BasicExport(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{DestDir: dir, Parallel: 2, SelfUsername: "self", SkipAttachments: true}
	convs := []keybase.ConvSummary{
		{ID: "c1", Channel: keybase.ChatChannel{Name: "self,alice", MembersType: "impteamnative"}},
		{ID: "c2", Channel: keybase.ChatChannel{Name: "self,bob", MembersType: "impteamnative"}},
	}
	lc := &mockListClient{convs: convs}

	newClient := func() (ClientAPI, error) {
		return &mockClient{msgs: []keybase.MsgSummary{
			{ID: 1, SentAtMs: 1000, Content: keybase.MsgContent{Type: "text"}},
		}}, nil
	}

	s, err := Run(cfg, lc, newClient)
	if err != nil {
		t.Fatal(err)
	}
	if s.Conversations != 2 {
		t.Errorf("conversations = %d, want 2", s.Conversations)
	}
	if s.Messages != 2 {
		t.Errorf("messages = %d, want 2", s.Messages)
	}
}

func TestRun_PanicRecovery(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{DestDir: dir, Parallel: 1, SelfUsername: "self", SkipAttachments: true}
	convs := []keybase.ConvSummary{
		{ID: "c1", Channel: keybase.ChatChannel{Name: "self,alice", MembersType: "impteamnative"}},
	}
	lc := &mockListClient{convs: convs}

	// Client that panics on ReadConversation
	newClient := func() (ClientAPI, error) {
		return &panicClient{}, nil
	}

	s, err := Run(cfg, lc, newClient)
	if err != nil {
		t.Fatal(err)
	}
	// Should have recovered from panic and recorded an error
	if s.Errors == 0 {
		t.Error("expected errors from panic recovery")
	}
}

type panicClient struct{}

func (p *panicClient) ReadConversation(convID string, known func(int) bool) ([]keybase.MsgSummary, error) {
	panic("test panic")
}
func (p *panicClient) GetMessages(_ string, _ []int) ([]keybase.MsgSummary, error) {
	return nil, nil
}
func (p *panicClient) DownloadAttachment(channel keybase.ChatChannel, msgID int, outPath string) error {
	return nil
}
func (p *panicClient) Close() error { return nil }
