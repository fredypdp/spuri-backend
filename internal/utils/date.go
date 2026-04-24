package utils

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

const isoDateLayout = "2006-01-02"

// Date representa uma data sem componente de hora (date-only).
// Serializa em JSON no formato ISO 8601: YYYY-MM-DD.
type Date struct {
	time.Time
}

func ParseDate(value string) (Date, error) {
	t, err := time.Parse(isoDateLayout, value)
	if err != nil {
		return Date{}, err
	}
	return Date{Time: t.UTC().Truncate(24 * time.Hour)}, nil
}

func (d Date) MarshalJSON() ([]byte, error) {
	if d.Time.IsZero() {
		return json.Marshal("")
	}
	return json.Marshal(d.Time.UTC().Format(isoDateLayout))
}

func (d *Date) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("data deve ser string no formato AAAA-MM-DD")
	}
	parsed, err := ParseDate(raw)
	if err != nil {
		return fmt.Errorf("formato de data inválido. Use AAAA-MM-DD")
	}
	d.Time = parsed.Time
	return nil
}

func (d *Date) Scan(value interface{}) error {
	switch v := value.(type) {
	case time.Time:
		d.Time = v.UTC().Truncate(24 * time.Hour)
		return nil
	case []byte:
		parsed, err := ParseDate(string(v))
		if err != nil {
			return err
		}
		d.Time = parsed.Time
		return nil
	case string:
		parsed, err := ParseDate(v)
		if err != nil {
			return err
		}
		d.Time = parsed.Time
		return nil
	default:
		return fmt.Errorf("não foi possível converter valor %T para Date", value)
	}
}

func (d Date) Value() (driver.Value, error) {
	if d.Time.IsZero() {
		return nil, nil
	}
	return d.Time.UTC().Format(isoDateLayout), nil
}
