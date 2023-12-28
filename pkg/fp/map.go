package fp

import "encoding/json"

func FromMap[T any](input map[string]any) (*T, error) {
	in, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	out := new(T)
	if err := json.Unmarshal(in, out); err != nil {
		return nil, err
	}

	return out, nil
}
