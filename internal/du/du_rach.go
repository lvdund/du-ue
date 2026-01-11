package du

import (
)

type RACHContext struct {
	dedicatedPreambles map[int]int64
	contensionBased    bool
}

var rachContext = &RACHContext{
	dedicatedPreambles: make(map[int]int64),
}

