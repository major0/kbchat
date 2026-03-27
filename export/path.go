package export

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/major0/keybase-export/keybase"
)

// ConvDirPath derives the export directory path from conversation metadata.
// DM/Group: <destDir>/Chats/<sorted_participants_minus_self>
// Team: <destDir>/Teams/<team_name>/<topic_name>
func ConvDirPath(destDir string, conv keybase.ConvSummary, selfUsername string) string {
	switch keybase.ClassifyConversation(conv.Channel) {
	case keybase.ConvTeam:
		topicName := conv.Channel.TopicName
		if topicName == "" {
			topicName = "general"
		}
		return filepath.Join(destDir, "Teams", conv.Channel.Name, topicName)
	default:
		parts := strings.Split(conv.Channel.Name, ",")
		var filtered []string
		for _, p := range parts {
			if p != selfUsername {
				filtered = append(filtered, p)
			}
		}
		sort.Strings(filtered)
		name := strings.Join(filtered, ",")
		if name == "" {
			name = selfUsername // self-chat
		}
		return filepath.Join(destDir, "Chats", name)
	}
}
