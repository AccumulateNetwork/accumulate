package main

import (
	"testing"
	"time"
)

func TestBlock_GetTotalStaked(t *testing.T) {
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Block{
				MajorHeight: tt.fields.MajorHeight,
				MinorHeight: tt.fields.MinorHeight,
				Timestamp:   tt.fields.Timestamp,
				Accounts:    tt.fields.Accounts,
			}
			if gotTotalTokens := b.GetTotalStaked(tt.args.Type); gotTotalTokens != tt.wantTotalTokens {
				t.Errorf("Block.GetTotalStaked() = %v, want %v", gotTotalTokens, tt.wantTotalTokens)
			}
		})
	}
}
