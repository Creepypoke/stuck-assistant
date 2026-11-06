package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
)

const Version = "preparation-v1"

var slug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

type Block struct {
	ID          string   `yaml:"id" json:"id"`
	Kind        string   `yaml:"kind" json:"kind"`
	HeadingPath []string `yaml:"heading_path,omitempty" json:"heading_path,omitempty"`
	Text        string   `yaml:"text" json:"text"`
	Start       int      `yaml:"start" json:"start"`
	End         int      `yaml:"end" json:"end"`
}
type Span struct {
	DocumentSHA256 string `yaml:"document_sha256" json:"document_sha256"`
	BlockID        string `yaml:"block_id" json:"block_id"`
	Start          int    `yaml:"start" json:"start"`
	End            int    `yaml:"end" json:"end"`
	Quote          string `yaml:"quote" json:"quote"`
}
type Fact struct {
	Quote   string `yaml:"quote" json:"quote"`
	BlockID string `yaml:"block_id" json:"block_id"`
}
type Card struct {
	ID               string   `yaml:"situation_id" json:"situation_id"`
	Episode          string   `yaml:"episode,omitempty" json:"episode,omitempty"`
	Branch           string   `yaml:"branch,omitempty" json:"branch,omitempty"`
	Location         string   `yaml:"location,omitempty" json:"location,omitempty"`
	Task             string   `yaml:"task" json:"task"`
	Conditions       []string `yaml:"conditions,omitempty" json:"conditions,omitempty"`
	RequiredItems    []string `yaml:"required_items,omitempty" json:"required_items,omitempty"`
	Aliases          []string `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Facts            []Fact   `yaml:"facts,omitempty" json:"facts,omitempty"`
	ExactCode        string   `yaml:"exact_code,omitempty" json:"exact_code,omitempty"`
	OrderedActions   []string `yaml:"ordered_actions,omitempty" json:"ordered_actions,omitempty"`
	Result           string   `yaml:"result,omitempty" json:"result,omitempty"`
	BlockIDs         []string `yaml:"block_ids" json:"block_ids"`
	SourceSpans      []Span   `yaml:"source_spans" json:"source_spans"`
	ValidationStatus string   `yaml:"validation_status" json:"validation_status"`
}
type Entry struct {
	SituationID string `yaml:"situation_id" json:"situation_id"`
	Section     string `yaml:"section" json:"section"`
	SearchText  string `yaml:"search_text" json:"search_text"`
	AnswerText  string `yaml:"answer_text" json:"answer_text"`
	SourceSpans []Span `yaml:"source_spans" json:"source_spans"`
}
type Manifest struct {
	AnnotationPromptSHA256    string `yaml:"annotation_prompt_sha256,omitempty" json:"annotation_prompt_sha256,omitempty"`
	AnnotationProtocol        string `yaml:"annotation_protocol,omitempty" json:"annotation_protocol,omitempty"`
	AnnotationMaxOutputTokens int    `yaml:"annotation_max_output_tokens,omitempty" json:"annotation_max_output_tokens,omitempty"`

	Version            string   `yaml:"version" json:"version"`
	GameID             string   `yaml:"game_id" json:"game_id"`
	GameName           string   `yaml:"game_name" json:"game_name"`
	InputName          string   `yaml:"input_name" json:"input_name"`
	RawSHA256          string   `yaml:"raw_sha256" json:"raw_sha256"`
	DocumentSHA256     string   `yaml:"document_sha256" json:"document_sha256"`
	AnnotationProvider string   `yaml:"annotation_provider,omitempty" json:"annotation_provider,omitempty"`
	AnnotationModel    string   `yaml:"annotation_model,omitempty" json:"annotation_model,omitempty"`
	MaxBatchChars      int      `yaml:"max_batch_chars" json:"max_batch_chars"`
	Warnings           []string `yaml:"warnings,omitempty" json:"warnings,omitempty"`
}
type Artifacts struct {
	Manifest Manifest
	Raw      []byte
	Document string
	Blocks   []Block
	Cards    []Card
	Entries  []Entry
}
