package protocol

import (
	"math/big"
	"strconv"
	"strings"
)

// FormatAmount formats a fixed point amount.
func FormatAmount(amount uint64, precision int) string {
	return formatAmount(strconv.FormatUint(amount, 10), precision)
}

// FormatBigAmount formats a fixed point amount.
func FormatBigAmount(amount *big.Int, precision int) string {
	return formatAmount(amount.Text(10), precision)
}

func formatAmount(amount string, precision int) string {
	// Add leading zeros to ensure the string is at least precision digits long
	if len(amount) < precision {
		amount = strings.Repeat("0", precision-len(amount)) + amount
	}

	// Separate into an integer and a fractional part
	ipart, fpart := amount[:len(amount)-precision], amount[len(amount)-precision:]

	// If the integer part is empty, add a leading 0
	if ipart == "" {
		ipart = "0"
	}

	// Trim trailing zeros from the fractional part
	fpart = strings.TrimRight(fpart, "0")

	// If the fractional part is not empty, add a decimal point
	if fpart != "" {
		fpart = "." + fpart
	}

	// Join the integer and fractional parts with a decimal point
	return ipart + fpart
}
