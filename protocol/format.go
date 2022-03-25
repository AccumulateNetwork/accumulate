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
	amount = strings.Repeat("0", len(amount)-precision) + amount

	// Separate into an integer and a fractional part
	ipart, fpart := amount[:len(amount)-precision], amount[len(amount)-precision:]

	// If the integer part is empty, add a leading 0
	if ipart == "" {
		ipart = "0"
	}

	// Join the integer and fractional parts with a decimal point
	return ipart + "." + fpart
}
