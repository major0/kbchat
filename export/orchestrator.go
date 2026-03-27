package export

import (
	"fmt"
	"log"
	"sync"

	"github.com/major0/keybase-export/keybase"
)

// Config holds export configuration.
type Config struct {
	DestDir         string
	Filters         []string
	Parallel        int
	Verbose         bool
	SkipAttachments bool
	SelfUsername     string
}

// Summary holds aggregate export results.
type Summary struct {
	Conversations int
	Messages      int
	Attachments   int
	Errors        int
}

// SummarizeResults computes aggregate totals from individual results.
func SummarizeResults(results []Result) Summary {
	var s Summary
	s.Conversations = len(results)
	for _, r := range results {
		s.Messages += r.MessagesExported
		s.Attachments += r.AttachmentsDownloaded
		s.Errors += len(r.Errors)
	}
	return s
}

// ListAPI abstracts the conversation listing method.
type ListAPI interface {
	ListConversations() ([]keybase.ConvSummary, error)
	Close() error
}

// ClientFactory creates new ClientAPI instances for workers.
type ClientFactory func() (ClientAPI, error)

// Run executes the full export: discover, filter, dispatch, collect, summarize.
func Run(cfg Config, listClient ListAPI, newClient ClientFactory) (Summary, error) {
	// Discover conversations
	convs, err := listClient.ListConversations()
	if err != nil {
		return Summary{}, fmt.Errorf("list conversations: %w", err)
	}
	listClient.Close()

	// Filter
	filtered := FilterConversations(convs, cfg.Filters, cfg.SelfUsername)
	if len(filtered) == 0 {
		return Summary{}, nil
	}

	// Dispatch to channel
	jobs := make(chan keybase.ConvSummary, len(filtered))
	for _, conv := range filtered {
		jobs <- conv
	}
	close(jobs)

	// Collect results
	var mu sync.Mutex
	var results []Result
	var wg sync.WaitGroup

	total := len(filtered)
	done := 0

	for i := 0; i < cfg.Parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Panic recovery
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					results = append(results, Result{Errors: []error{fmt.Errorf("panic: %v", r)}})
					mu.Unlock()
				}
			}()

			// Each worker creates its own client
			client, err := newClient()
			if err != nil {
				mu.Lock()
				results = append(results, Result{Errors: []error{fmt.Errorf("create client: %w", err)}})
				mu.Unlock()
				return
			}

			for conv := range jobs {
				result := ExportConversation(client, conv, cfg.DestDir, cfg.SelfUsername, cfg.SkipAttachments, cfg.Verbose)

				mu.Lock()
				results = append(results, result)
				done++
				log.Printf("[%d/%d] %s: %d messages, %d attachments, %d errors",
					done, total, conv.ID, result.MessagesExported, result.AttachmentsDownloaded, len(result.Errors))
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	s := SummarizeResults(results)
	log.Printf("Export complete: %d conversations, %d messages, %d attachments, %d errors",
		s.Conversations, s.Messages, s.Attachments, s.Errors)
	return s, nil
}
