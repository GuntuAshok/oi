package main

import (
	"fmt"
	"strings"

	"github.com/GuntuAshok/oi/internal/proto"
)

func (m *Mods) setupStreamContext(content string, mod Model) error {
	cfg := m.Config
	m.messages = []proto.Message{} // 1. Reset messages

	// 2. Attempt to load history from cache FIRST.
	if !cfg.NoCache && cfg.cacheReadFromID != "" {
		if err := m.cache.Read(cfg.cacheReadFromID, &m.messages); err != nil {
			return modsError{
				err: err,
				reason: fmt.Sprintf(
					"There was a problem reading the cache. Use %s / %s to disable it.",
					m.Styles.InlineCode.Render("--no-cache"),
					m.Styles.InlineCode.Render("NO_CACHE"),
				),
			}
		}
	}

	// 3. Only add system/role prompts if this is a NEW conversation
	//    (i.e., no messages were loaded from cache).
	if len(m.messages) == 0 {
		if txt := cfg.FormatText[cfg.FormatAs]; cfg.Format && txt != "" {
			m.messages = append(m.messages, proto.Message{
				Role:    proto.RoleSystem,
				Content: txt,
			})
		}

		if cfg.Role != "" {
			roleSetup, ok := cfg.Roles[cfg.Role]
			if !ok {
				return modsError{
					err:    fmt.Errorf("role %q does not exist", cfg.Role),
					reason: "Could not use role",
				}
			}
			for _, msg := range roleSetup {
				content, err := loadMsg(msg)
				if err != nil {
					return modsError{
						err:    err,
						reason: "Could not use role",
					}
				}
				m.messages = append(m.messages, proto.Message{
					Role:    proto.RoleSystem,
					Content: content,
				})
			}
		}
	}

	// 4. Combine prefix (from args) and content (from stdin) for the user message
	if prefix := cfg.Prefix; prefix != "" {
		content = strings.TrimSpace(prefix + "\n\n" + content)
	}

	// 5. Apply character limit if any
	if !cfg.NoLimit && int64(len(content)) > mod.MaxChars {
		content = content[:mod.MaxChars]
	}

	// 6. Append the new user message to the (potentially loaded) history.
	m.messages = append(m.messages, proto.Message{
		Role:    proto.RoleUser,
		Content: content,
	})

	return nil
}
