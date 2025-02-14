package common

type APISecretStats struct {
	FileSize    int64 `json:"file_size"`
	ActiveCount int   `json:"active_count"`
	TrashCount  int   `json:"trash_count"`
}
