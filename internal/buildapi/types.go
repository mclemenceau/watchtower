package buildapi

// Artefact mirrors the Test Observer API ArtefactResponse for the image family.
// Only fields used by ARGUS are included; extra API fields are silently discarded.
type Artefact struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Version  string `json:"version"` // YYYYMMDD build date
	OS       string `json:"os"`
	Release  string `json:"release"`
	Stage    string `json:"stage"`  // pending | current
	Status   string `json:"status"` // APPROVED | MARKED_AS_FAILED | UNDECIDED
	Archived bool   `json:"archived"`
	ImageURL string `json:"image_url"`
}

type ChangeReport struct {
	NewFailures  []ArtefactDelta `json:"new_failures"`
	Recoveries   []ArtefactDelta `json:"recoveries"`
	OtherChanges []ArtefactDelta `json:"other_changes"`
	NewArtefacts []Artefact      `json:"new_artefacts"`
}

type ArtefactDelta struct {
	Name      string `json:"name"`
	Release   string `json:"release"`
	Version   string `json:"version"`
	OldStatus string `json:"old_status"`
	NewStatus string `json:"new_status"`
}
