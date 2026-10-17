package adapters

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/msitarzewski/agent-room/internal/domain"
)

type Context struct {
	ProjectID      string
	ActorID        string
	AgentID        string
	RunID          string
	SessionID      string
	SourceSystem   string
	NativeEventID  string
	SourceSequence int64
	Now            func() time.Time
	NewID          func() string
}

var metadataToken = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]*$`)

// MetadataToken accepts bounded identifier syntax, never free-form messages.
func MetadataToken(value string, maximum int) string {
	if len(value) > maximum || !metadataToken.MatchString(value) {
		return ""
	}
	return value
}

// UsageCounts extracts only known nonnegative integer counters from native data.
func UsageCounts(raw json.RawMessage) map[string]int64 {
	var input map[string]json.RawMessage
	_ = json.Unmarshal(raw, &input)
	result := map[string]int64{}
	for _, key := range []string{"input_tokens", "output_tokens", "cached_input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens", "total_tokens"} {
		var value int64
		if data, ok := input[key]; ok && string(data) != "null" && json.Unmarshal(data, &value) == nil && value >= 0 && value <= 1_000_000_000_000 {
			result[key] = value
		}
	}
	return result
}

func SourceSequence(base int64, offset int64) (int64, error) {
	if base < 0 || offset <= 0 || base > math.MaxInt64-offset {
		return 0, fmt.Errorf("invalid adapter source sequence")
	}
	return base + offset, nil
}

func (c Context) Event(eventType string, subjectType domain.ResourceType, subjectID string, payload any, occurredAt time.Time) (domain.Event, error) {
	if c.ProjectID == "" || c.ActorID == "" {
		return domain.Event{}, fmt.Errorf("adapter project and actor are required")
	}
	if occurredAt.IsZero() {
		if c.Now != nil {
			occurredAt = c.Now()
		} else {
			occurredAt = time.Now().UTC()
		}
	}
	id := ""
	if c.NewID != nil {
		id = c.NewID()
	} else {
		id = randomID()
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return domain.Event{}, err
	}
	event := domain.Event{ID: id, ProjectID: c.ProjectID, Type: eventType, SubjectType: subjectType, SubjectID: subjectID, ActorID: c.ActorID, OccurredAt: occurredAt.UTC(), SchemaVersion: 1, Payload: raw, SourceSystem: c.SourceSystem, SourceEventID: c.NativeEventID, SourceSequence: c.SourceSequence}
	if event.SourceSystem != "" && event.SourceEventID == "" {
		event.SourceEventID = StableSourceID(event.SourceSystem, eventType, subjectID, string(raw))
	}
	return event, nil
}

func StableSourceID(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

func randomID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}
