package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type Paths []string

func (p Paths) Value() (driver.Value, error) {
	return json.Marshal([]string(p))
}

func (p *Paths) Scan(value interface{}) error {
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, (*[]string)(p))
	case string:
		return json.Unmarshal([]byte(v), (*[]string)(p))
	case nil:
		*p = nil
		return nil
	default:
		return fmt.Errorf("cannot scan %T", value)
	}
}
