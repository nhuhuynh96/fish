package mqtt

import (
	"reflect"
	"testing"
)

func TestExpandMeasureActions(t *testing.T) {
	tests := []struct {
		name    string
		sensors []string
		want    []string
	}{
		{name: "empty is all", sensors: nil, want: []string{"ph", "temp", "tds"}},
		{name: "all", sensors: []string{"all"}, want: []string{"ph", "temp", "tds"}},
		{name: "ui temp maps to temp", sensors: []string{"ph", "temperature", "tds"}, want: []string{"ph", "temp", "tds"}},
		{name: "temp with ph", sensors: []string{"temp", "ph"}, want: []string{"temp", "ph"}},
		{name: "turbidity ignored", sensors: []string{"ph", "turbidity"}, want: []string{"ph"}},
		{name: "dedupe", sensors: []string{"ph", "ph", "temp"}, want: []string{"ph", "temp"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandMeasureActions(tt.sensors)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}
