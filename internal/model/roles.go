package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type Roles []int

func (r Roles) Value() (driver.Value, error) {
	return json.Marshal([]int(r))
}

func (r *Roles) Scan(value interface{}) error {
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, (*[]int)(r))
	case string:
		return json.Unmarshal([]byte(v), (*[]int)(r))
	case nil:
		*r = nil
		return nil
	default:
		return fmt.Errorf("cannot scan %T", value)
	}
}

func (r Roles) Contains(role int) bool {
	for _, v := range r {
		if v == role {
			return true
		}
	}
	return false
}
