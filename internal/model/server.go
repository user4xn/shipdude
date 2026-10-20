package model

// Server represents a target VPS node managed by ShipDude. The SSH private
// key is stored encrypted at rest and decrypted on read by the repository.
type Server struct {
	ID            int64  `gorm:"column:id;primaryKey;autoIncrement" column:"id" json:"id"`
	Name          string `gorm:"column:name;not null" column:"name" json:"name"`
	IPAddress     string `gorm:"column:ip_address;not null" column:"ip_address" json:"ip_address"`
	SSHPort       int    `gorm:"column:ssh_port;default:22" column:"ssh_port" json:"ssh_port"`
	SSHUser       string `gorm:"column:ssh_user;default:root" column:"ssh_user" json:"ssh_user"`
	SSHPrivateKey string `gorm:"column:ssh_private_key;not null" column:"ssh_private_key" json:"ssh_private_key"`
}

// TableName returns the physical table for servers.
func (Server) TableName() string {
	return "servers"
}
