package store

import (
	"os"
	"path/filepath"
)

// ConvInfo represents a conversation discovered from the on-disk export store.
type ConvInfo struct {
	Type     string // "Chat" or "Team"
	Name     string // participant list or team name
	Channel  string // topic name (Teams only, empty for Chats)
	Dir      string // absolute path to conversation directory
	MsgCount int    // number of messages/<id>/ directories
}

// ScanConversations walks <storePath>/Chats/ and <storePath>/Teams/ to
// discover conversations. For each conversation directory it counts
// messages/<id>/ subdirectories.
func ScanConversations(storePath string) ([]ConvInfo, error) {
	absStore, err := filepath.Abs(storePath)
	if err != nil {
		return nil, err
	}

	var convs []ConvInfo

	// Scan Chats/: each subdirectory is a conversation.
	chats, err := scanChats(absStore)
	if err != nil {
		return nil, err
	}
	convs = append(convs, chats...)

	// Scan Teams/: each subdirectory is a team, each sub-subdirectory is a channel.
	teams, err := scanTeams(absStore)
	if err != nil {
		return nil, err
	}
	convs = append(convs, teams...)

	return convs, nil
}

// scanChats reads <storePath>/Chats/ and returns one ConvInfo per subdirectory.
func scanChats(storePath string) ([]ConvInfo, error) {
	chatsDir := filepath.Join(storePath, "Chats")
	entries, err := os.ReadDir(chatsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var convs []ConvInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(chatsDir, e.Name())
		convs = append(convs, ConvInfo{
			Type:     "Chat",
			Name:     e.Name(),
			Dir:      dir,
			MsgCount: countMessages(dir),
		})
	}
	return convs, nil
}

// scanTeams reads <storePath>/Teams/ and returns one ConvInfo per
// team/channel subdirectory pair.
func scanTeams(storePath string) ([]ConvInfo, error) {
	teamsDir := filepath.Join(storePath, "Teams")
	teamEntries, err := os.ReadDir(teamsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var convs []ConvInfo
	for _, te := range teamEntries {
		if !te.IsDir() {
			continue
		}
		teamDir := filepath.Join(teamsDir, te.Name())
		chanEntries, err := os.ReadDir(teamDir)
		if err != nil {
			return nil, err
		}
		for _, ce := range chanEntries {
			if !ce.IsDir() {
				continue
			}
			dir := filepath.Join(teamDir, ce.Name())
			convs = append(convs, ConvInfo{
				Type:     "Team",
				Name:     te.Name(),
				Channel:  ce.Name(),
				Dir:      dir,
				MsgCount: countMessages(dir),
			})
		}
	}
	return convs, nil
}

// countMessages counts the number of subdirectories in <convDir>/messages/.
func countMessages(convDir string) int {
	msgsDir := filepath.Join(convDir, "messages")
	entries, err := os.ReadDir(msgsDir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() {
			count++
		}
	}
	return count
}
