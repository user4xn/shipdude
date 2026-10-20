package model

// Setting is an encrypted key/value configuration row.
type Setting struct {
	Key   string `gorm:"column:key;primaryKey" column:"key" json:"key"`
	Value string `gorm:"column:value;not null" column:"value" json:"value"`
}

// TableName returns the physical table for settings.
func (Setting) TableName() string {
	return "settings"
}
