package vmm

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPutThenGetContainerValue(t *testing.T) {
	testCases := []struct {
		container     string
		key           string
		wantValue     string
		wantErrString string
	}{
		{containerName, "key123", "value123", ""},                           // success case
		{"aaa", "key123", "", "bucket aaa not found"},                       // invalid container
		{containerName, "aaa", "", "key aaa not found in " + containerName}, // invalid key
	}

	err := v.KVStore.PutContainterValue(containerName, []KeyValue{{"key123", "value123"}})
	assert.Nil(t, err)
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("get %s from %s", tc.key, tc.container), func(t *testing.T) {
			value, err := v.KVStore.GetContainerValue(tc.container, tc.key)
			assert.Equal(t, value, tc.wantValue)
			if tc.wantErrString == "" {
				assert.Nil(t, err)
			} else {
				assert.EqualError(t, err, tc.wantErrString)
			}
		})
	}
}
