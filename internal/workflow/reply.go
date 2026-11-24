package workflow

import (
	"strings"
)

// 实例用标记与宿主沟通。标记只存在于实例回复，不进入投影文本。
const (
	QuestionMarker = "<<workflow:question>>"
	HoleMarker     = "<<workflow:hole>>"
	ArtifactMarker = "<<workflow:artifact>>"
)

// ArtifactDraft 是实例发布的一个交付物草稿。名称与内容用竖线分隔。
type ArtifactDraft struct {
	Name    string
	Content string
}

// ActorReply 是一次实例回复被解析后的结构。
type ActorReply struct {
	Text      string
	Question  string
	Hole      string
	Artifacts []ArtifactDraft
}

// ParseActorReply 拆出升级问题、留白和产物。产物不等于最终回复本身：
// 只有显式标记的内容才是交付物。
func ParseActorReply(text string) ActorReply {
	reply := ActorReply{}
	for {
		value, rest, found := takeMarked(text, ArtifactMarker)
		if !found {
			break
		}
		text = rest
		name, content, _ := strings.Cut(strings.TrimSpace(value), "|")
		if title, body, multiline := strings.Cut(strings.TrimSpace(value), "\n"); multiline && !strings.Contains(title, "|") {
			name, content = title, body
		}
		name, content = strings.TrimSpace(name), strings.TrimSpace(content)
		if content == "" {
			continue
		}
		if name == "" {
			name = content
		}
		reply.Artifacts = append(reply.Artifacts, ArtifactDraft{Name: name, Content: content})
	}
	if value, rest, found := takeMarked(text, QuestionMarker); found {
		reply.Question, text = strings.TrimSpace(value), rest
	}
	if value, rest, found := takeMarked(text, HoleMarker); found {
		reply.Hole, text = strings.TrimSpace(value), rest
	}
	reply.Text = strings.TrimSpace(text)
	return reply
}

// takeMarked 取出一段由同一标记包裹的内容。
func takeMarked(text, marker string) (value, rest string, found bool) {
	start := strings.Index(text, marker)
	if start < 0 {
		return "", text, false
	}
	end := strings.Index(text[start+len(marker):], marker)
	if end < 0 {
		return "", text, false
	}
	value = text[start+len(marker) : start+len(marker)+end]
	before, after := text[:start], text[start+len(marker)+end+len(marker):]
	return value, strings.TrimSpace(before + "\n" + after), true
}
