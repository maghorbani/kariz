package models

import "encoding/json"

// VolumeMountList is a slice of VolumeMount that implements sql.Scanner and driver.Valuer
// for JSONB column storage.
type VolumeMountList []VolumeMount

// Scan implements the sql.Scanner interface for reading JSONB from the database.
func (v *VolumeMountList) Scan(src interface{}) error {
	if src == nil {
		*v = []VolumeMount{}
		return nil
	}
	var data []byte
	switch val := src.(type) {
	case []byte:
		data = val
	case string:
		data = []byte(val)
	default:
		return nil
	}
	return json.Unmarshal(data, v)
}

// Value implements the driver.Valuer interface for writing JSONB to the database.
func (v VolumeMountList) Value() (interface{}, error) {
	return json.Marshal(v)
}

// ArtifactDeclareList is a slice of ArtifactDeclare that implements sql.Scanner and driver.Valuer
// for JSONB column storage.
type ArtifactDeclareList []ArtifactDeclare

// Scan implements the sql.Scanner interface for reading JSONB from the database.
func (a *ArtifactDeclareList) Scan(src interface{}) error {
	if src == nil {
		*a = []ArtifactDeclare{}
		return nil
	}
	var data []byte
	switch val := src.(type) {
	case []byte:
		data = val
	case string:
		data = []byte(val)
	default:
		return nil
	}
	return json.Unmarshal(data, a)
}

// Value implements the driver.Valuer interface for writing JSONB to the database.
func (a ArtifactDeclareList) Value() (interface{}, error) {
	return json.Marshal(a)
}
