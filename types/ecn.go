package types

import "fmt"

type ECN uint8

const (
	ECNUnsupported ECN = iota
	ECNNon             // 00
	ECT1               // 01
	ECT0               // 10
	ECNCE              // 11
)

func ParseECNHeaderBits(bits byte) ECN {
	switch bits {
	case 0:
		return ECNNon
	case 0b00000010:
		return ECT0
	case 0b00000001:
		return ECT1
	case 0b00000011:
		return ECNCE
	default:
		panic("invalid ECN bits")
	}
}

func (e ECN) ToHeaderBits() byte {
	//nolint:exhaustive // There are only 4 values.
	switch e {
	case ECNNon:
		return 0
	case ECT0:
		return 0b00000010
	case ECT1:
		return 0b00000001
	case ECNCE:
		return 0b00000011
	default:
		panic("ECN unsupported")
	}
}

func (e ECN) String() string {
	switch e {
	case ECNUnsupported:
		return "ECN unsupported"
	case ECNNon:
		return "Not-ECT"
	case ECT1:
		return "ECT(1)"
	case ECT0:
		return "ECT(0)"
	case ECNCE:
		return "CE"
	default:
		return fmt.Sprintf("invalid ECN value: %d", e)
	}
}
