package restic

import "fmt"

type StatusUpdate struct {
	MessageType      string   `json:"message_type"` // "status"
	SecondsElapsed   uint64   `json:"seconds_elapsed,omitempty"`
	SecondsRemaining uint64   `json:"seconds_remaining,omitempty"`
	PercentDone      float64  `json:"percent_done"`
	TotalFiles       uint64   `json:"total_files,omitempty"`
	FilesDone        uint64   `json:"files_done,omitempty"`
	TotalBytes       uint64   `json:"total_bytes,omitempty"`
	BytesDone        uint64   `json:"bytes_done,omitempty"`
	ErrorCount       uint     `json:"error_count,omitempty"`
	CurrentFiles     []string `json:"current_files,omitempty"`
}

func (s *StatusUpdate) GetPercentDone() string {
	return fmt.Sprintf("%.2f%%", s.PercentDone*100)
}

type ErrorObject struct {
	Message string `json:"message"`
}

type ErrorUpdate struct {
	MessageType string      `json:"message_type"` // "error"
	Error       ErrorObject `json:"error"`
	During      string      `json:"during"`
	Item        string      `json:"item"`
}

type VerboseUpdate struct {
	MessageType        string  `json:"message_type"` // "verbose_status"
	Action             string  `json:"action"`
	Item               string  `json:"item"`
	Duration           float64 `json:"duration"` // in seconds
	DataSize           uint64  `json:"data_size"`
	DataSizeInRepo     uint64  `json:"data_size_in_repo"`
	MetadataSize       uint64  `json:"metadata_size"`
	MetadataSizeInRepo uint64  `json:"metadata_size_in_repo"`
	TotalFiles         uint    `json:"total_files"`
}

type SummaryOutput struct {
	MessageType         string  `json:"message_type"` // "summary"
	FilesNew            uint    `json:"files_new"`
	FilesChanged        uint    `json:"files_changed"`
	FilesUnmodified     uint    `json:"files_unmodified"`
	DirsNew             uint    `json:"dirs_new"`
	DirsChanged         uint    `json:"dirs_changed"`
	DirsUnmodified      uint    `json:"dirs_unmodified"`
	DataBlobs           int     `json:"data_blobs"`
	TreeBlobs           int     `json:"tree_blobs"`
	DataAdded           uint64  `json:"data_added"`
	DataAddedPacked     uint64  `json:"data_added_packed"`
	TotalFilesProcessed uint    `json:"total_files_processed"`
	TotalBytesProcessed uint64  `json:"total_bytes_processed"`
	TotalDuration       float64 `json:"total_duration"` // in seconds
	SnapshotID          string  `json:"snapshot_id,omitempty"`
	DryRun              bool    `json:"dry_run,omitempty"`
}

type Snapshot struct {
	Time           string           `json:"time"`
	Tree           string           `json:"tree"`
	Paths          []string         `json:"paths"`
	Hostname       string           `json:"hostname"`
	Username       string           `json:"username"`
	ProgramVersion string           `json:"program_version"`
	Summary        *SnapshotSummary `json:"summary"`
	Id             string           `json:"id"`
	ShortId        string           `json:"short_id"`
}

type SnapshotSummary struct {
	BackupStart         string `json:"backup_start"`
	BackupEnd           string `json:"backup_end"`
	FilesNew            int64  `json:"files_new"`
	FilesChanged        int64  `json:"files_changed"`
	FilesUnmodified     int64  `json:"files_unmodified"`
	DirsNew             int64  `json:"dirs_new"`
	DirsChanged         int64  `json:"dirs_changed"`
	DirsUnmodified      int64  `json:"dirs_unmodified"`
	DataBlobs           int64  `json:"data_blobs"`
	TreeBlobs           int64  `json:"tree_blobs"`
	DataAdded           int64  `json:"data_added"`
	DataAddedPacked     int64  `json:"data_added_packed"`
	TotalFilesProcessed int64  `json:"total_files_processed"`
	TotalBytesProcessed int64  `json:"total_bytes_processed"`
}

type RestoreStatusUpdate struct {
	MessageType    string  `json:"message_type"` // "status"
	SecondsElapsed uint64  `json:"seconds_elapsed,omitempty"`
	PercentDone    float64 `json:"percent_done"`
	TotalFiles     uint64  `json:"total_files,omitempty"`
	FilesRestored  uint64  `json:"files_restored,omitempty"`
	FilesSkipped   uint64  `json:"files_skipped,omitempty"`
	TotalBytes     uint64  `json:"total_bytes,omitempty"`
	BytesRestored  uint64  `json:"bytes_restored,omitempty"`
	BytesSkipped   uint64  `json:"bytes_skipped,omitempty"`
}

func (s *RestoreStatusUpdate) GetPercentDone() string {
	return fmt.Sprintf("%.2f%%", s.PercentDone*100)
}

type RestoreVerboseUpdate struct {
	MessageType string `json:"message_type"` // "verbose_status"
	Action      string `json:"action"`
	Item        string `json:"item"`
	Size        uint64 `json:"size"`
}

type RestoreSummaryOutput struct {
	MessageType    string `json:"message_type"` // "summary"
	SecondsElapsed uint64 `json:"seconds_elapsed,omitempty"`
	TotalFiles     uint64 `json:"total_files,omitempty"`
	FilesRestored  uint64 `json:"files_restored,omitempty"`
	FilesSkipped   uint64 `json:"files_skipped,omitempty"`
	TotalBytes     uint64 `json:"total_bytes,omitempty"`
	BytesRestored  uint64 `json:"bytes_restored,omitempty"`
	BytesSkipped   uint64 `json:"bytes_skipped,omitempty"`
}

type InitSummaryOutput struct {
	MessageType string `json:"message_type"` // "initialized"
	Id          string `json:"id"`
	Repository  string `json:"repository"`
}
