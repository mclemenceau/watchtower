package buildapi

// Artefact mirrors the Test Observer API ArtefactResponse for the image family.
// Only fields used by ARGUS are included; extra API fields are silently discarded.
type Artefact struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Version  string `json:"version"` // YYYYMMDD or YYYYMMDD.N (respin); today's date means build succeeded and image is available for testing
	OS       string `json:"os"`
	Release  string `json:"release"`
	Stage    string `json:"stage"`  // pending | current — pipeline release stage, not build state
	Status   string `json:"status"` // APPROVED | MARKED_AS_FAILED | UNDECIDED — test review state, unrelated to build availability
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
