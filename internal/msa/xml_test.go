package msa

import "testing"

func TestStatusSuccess(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{
			name: "success response-type",
			status: Status{
				ResponseType:        "Success",
				ResponseTypeNumeric: 0,
				ReturnCode:          1,
			},
			want: true,
		},
		{
			name: "info response-type with return-code 0",
			status: Status{
				ResponseType:        "Info",
				ResponseTypeNumeric: 2,
				ReturnCode:          0,
			},
			want: true,
		},
		{
			name: "error response-type",
			status: Status{
				ResponseType:        "Error",
				ResponseTypeNumeric: 1,
				ReturnCode:          0,
			},
			want: false,
		},
		{
			name: "info response-type nonzero return-code",
			status: Status{
				ResponseType:        "Info",
				ResponseTypeNumeric: 2,
				ReturnCode:          5,
			},
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.status.Success(); got != test.want {
				t.Fatalf("unexpected success result: got %v, want %v", got, test.want)
			}
		})
	}
}
