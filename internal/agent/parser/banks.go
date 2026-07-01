// internal/agent/parser/banks.go
package parser

import (
	"fmt"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"
)

// ── ICICI Credit Card ───────────────────────────────────────────────────────
// "INR 453.00 spent using ICICI Bank Card XX6009 on 03-Jun-26 on AMAZON PAY IN G."

var iciciCCRe = regexp.MustCompile(
	`(?i)INR\s+([\d,]+(?:\.\d{1,2})?)\s+spent using .* on (\d{2}-\w{3}-\d{2}) on ([^.]+)`)

func ExtractIciciCC(sms string) (merchant, rawDate, rawAmt string, isDebit bool, err error) {
	m := iciciCCRe.FindStringSubmatch(sms)
	if m == nil {
		log.Debugf("icici_cc: regex no match — pattern expects 'INR <amt> spent using ... on <DD-Mon-YY> on <merchant>'")
		return "", "", "", false, fmt.Errorf("icici_cc: no match")
	}
	log.Debugf("icici_cc: extracted rawAmt=%q rawDate=%q merchant=%q", m[1], m[2], strings.TrimSpace(m[3]))
	return strings.TrimSpace(m[3]), m[2], m[1], true, nil
}

// ── HDFC Debit Card ─────────────────────────────────────────────────────────
// "Spent! INR INR 215 On HDFC Bank Card 2148 At RAZ*Swiggy/Bangalore On 03 Jun,2026 01:18 PM IST"

var hdfcDebitRe = regexp.MustCompile(
	`(?i)Spent!\s+INR\s+INR\s+([\d,]+(?:\.\d{1,2})?)\s+On\s+HDFC\s+Bank\s+Card\s+\d+\s+At\s+(.+?)\s+On\s+(\d{1,2}\s+\w{3},\d{4})`)

func ExtractHdfcDebit(sms string) (merchant, rawDate, rawAmt string, isDebit bool, err error) {
	m := hdfcDebitRe.FindStringSubmatch(sms)
	if m == nil {
		log.Debugf("hdfc_debit: regex no match — pattern expects 'Spent! INR INR <amt> On HDFC Bank Card ... At <merchant> On <DD Mon,YYYY>'")
		return "", "", "", false, fmt.Errorf("hdfc_debit: no match")
	}
	log.Debugf("hdfc_debit: extracted rawAmt=%q rawDate=%q merchant=%q", m[1], m[3], strings.TrimSpace(m[2]))
	return strings.TrimSpace(m[2]), m[3], m[1], true, nil
}

// ── HDFC Credit Card ────────────────────────────────────────────────────────
// "Spent Rs.341 On HDFC Bank Card 2527 At ZEPTO MARKETPLACE PRIV On 2026-05-21:07:32:56."

var hdfcCCRe = regexp.MustCompile(
	`(?i)Spent\s+Rs\.?([\d,]+(?:\.\d{1,2})?)\s+On\s+HDFC\s+Bank\s+Card\s+\d+\s+At\s+(.+?)\s+On\s+(\d{4}-\d{2}-\d{2})`)

func ExtractHdfcCC(sms string) (merchant, rawDate, rawAmt string, isDebit bool, err error) {
	m := hdfcCCRe.FindStringSubmatch(sms)
	if m == nil {
		log.Debugf("hdfc_cc: regex no match — pattern expects 'Spent Rs.<amt> On HDFC Bank Card ... At <merchant> On <YYYY-MM-DD>'")
		return "", "", "", false, fmt.Errorf("hdfc_cc: no match")
	}
	log.Debugf("hdfc_cc: extracted rawAmt=%q rawDate=%q merchant=%q", m[1], m[3], strings.TrimSpace(m[2]))
	return strings.TrimSpace(m[2]), m[3], m[1], true, nil
}

// ── Axis Bank Checking ───────────────────────────────────────────────────────
// "INR 1804.05 debited\nA/c no. XX6386\n03-06-26, 10:21:54\nUPI/P2M/102154212206/IRCTC Rail Web\n..."

var axisChkAmtRe = regexp.MustCompile(`(?i)INR\s+([\d,]+(?:\.\d{1,2})?)\s+(debited|credited)`)
var axisChkAmtReB = regexp.MustCompile(`(?im)^(Debit|Credit)\s+INR\s+([\d,]+(?:\.\d{1,2})?)`)
var axisChkDateRe = regexp.MustCompile(`\b(\d{2}-\d{2}-\d{2})\b`)
var axisChkUPIRe = regexp.MustCompile(`(?i)(UPI/[^\n]+)`)

func ExtractAxisChecking(sms string) (merchant, rawDate, rawAmt string, isDebit bool, err error) {
	// Format A: "INR X debited/credited\nA/c no. XX...\nDD-MM-YY, HH:MM:SS\nUPI/P2M/.../Merchant"
	if amtM := axisChkAmtRe.FindStringSubmatch(sms); amtM != nil {
		dateM := axisChkDateRe.FindStringSubmatch(sms)
		if dateM == nil {
			log.Debugf("axis_checking: no date match — rawAmt=%q; pattern expects DD-MM-YY", amtM[1])
			return "", "", "", false, fmt.Errorf("axis_checking: no date")
		}
		merchantStr := ""
		if upiM := axisChkUPIRe.FindStringSubmatch(sms); upiM != nil {
			merchantStr = extractUPIMerchant(upiM[1])
			log.Debugf("axis_checking: UPI ref=%q → merchant=%q", upiM[1], merchantStr)
		} else {
			log.Debugf("axis_checking: no UPI ref found — merchant will be empty (LLM fallback)")
		}
		debit := strings.ToLower(amtM[2]) == "debited"
		log.Debugf("axis_checking/A: rawAmt=%q rawDate=%q isDebit=%v merchant=%q", amtM[1], dateM[1], debit, merchantStr)
		return merchantStr, dateM[1], amtM[1], debit, nil
	}

	// Format B: "Debit INR X\nAxis Bank A/c XX...\nDD-MM-YY HH:MM:SS\nMerchant line\n..."
	if amtM := axisChkAmtReB.FindStringSubmatch(sms); amtM != nil {
		debit := strings.EqualFold(amtM[1], "Debit")
		lines := strings.Split(strings.TrimSpace(sms), "\n")
		merchantStr := ""
		for i, line := range lines {
			if m := axisDateLineRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
				rawDate = m[1]
				if i+1 < len(lines) {
					merchantStr = strings.TrimSpace(lines[i+1])
				}
				break
			}
		}
		if rawDate == "" {
			log.Debugf("axis_checking: format B no date found — rawAmt=%q", amtM[2])
			return "", "", "", false, fmt.Errorf("axis_checking: no date in format B")
		}
		log.Debugf("axis_checking/B: rawAmt=%q rawDate=%q isDebit=%v merchant=%q", amtM[2], rawDate, debit, merchantStr)
		return merchantStr, rawDate, amtM[2], debit, nil
	}

	log.Debugf("axis_checking: no match — format A expects 'INR <amt> debited/credited', format B expects 'Debit/Credit INR <amt>'")
	return "", "", "", false, fmt.Errorf("axis_checking: no match")
}

// extractUPIMerchant returns the human-readable part after the numeric reference ID.
// "UPI/P2M/102154212206/IRCTC Rail Web" → "IRCTC Rail Web"
func extractUPIMerchant(upiRef string) string {
	parts := strings.Split(upiRef, "/")
	for i, p := range parts {
		if isAllDigits(p) && i+1 < len(parts) {
			return strings.Join(parts[i+1:], " ")
		}
	}
	return parts[len(parts)-1]
}

func isAllDigits(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// ── Axis Bank Credit Card ────────────────────────────────────────────────────
// "Spent INR 210.12\nAxis Bank Card no. XX1610\n08-05-26 18:44:17 IST\nDISTRICT MO\nAvl Limit: ..."

var axisSpentRe = regexp.MustCompile(`(?i)^Spent\s+INR\s+([\d,]+(?:\.\d{1,2})?)`)
var axisDateLineRe = regexp.MustCompile(`^(\d{2}-\d{2}-\d{2})\s`)

func ExtractAxisCC(sms string) (merchant, rawDate, rawAmt string, isDebit bool, err error) {
	amtM := axisSpentRe.FindStringSubmatch(sms)
	if amtM == nil {
		log.Debugf("axis_cc: no amount match — pattern expects SMS to start with 'Spent INR <amt>'")
		return "", "", "", false, fmt.Errorf("axis_cc: no amount")
	}

	lines := strings.Split(strings.TrimSpace(sms), "\n")
	merchantLineIdx := -1
	for i, line := range lines {
		if m := axisDateLineRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			rawDate = m[1]
			merchantLineIdx = i + 1
			break
		}
	}
	if rawDate == "" || merchantLineIdx < 0 || merchantLineIdx >= len(lines) {
		log.Debugf("axis_cc: no date/merchant line found — rawAmt=%q, %d lines scanned", amtM[1], len(lines))
		return "", "", "", false, fmt.Errorf("axis_cc: no date or merchant line")
	}

	merchantStr := strings.TrimSpace(lines[merchantLineIdx])
	log.Debugf("axis_cc: rawAmt=%q rawDate=%q merchant=%q", amtM[1], rawDate, merchantStr)
	return merchantStr, rawDate, amtM[1], true, nil
}

// ── Axis Bank UPI Debit ──────────────────────────────────────────────────────
// "Your A/c has been debited towards NETFLIX for INR 649.00 on 22-06-26. UPI_ID@okaxis - Axis Bank"

var axisUPIRe = regexp.MustCompile(
	`(?i)Your A/c has been debited towards (.+?) for INR\s*([\d,]+(?:\.\d{1,2})?) on (\d{2}-\d{2}-\d{2})`)

func ExtractAxisUPI(sms string) (merchant, rawDate, rawAmt string, isDebit bool, err error) {
	m := axisUPIRe.FindStringSubmatch(sms)
	if m == nil {
		log.Debugf("axis_upi: no match — pattern expects 'Your A/c has been debited towards <merchant> for INR <amt> on DD-MM-YY'")
		return "", "", "", false, fmt.Errorf("axis_upi: no match")
	}
	log.Debugf("axis_upi: merchant=%q rawAmt=%q rawDate=%q", strings.TrimSpace(m[1]), m[2], m[3])
	return strings.TrimSpace(m[1]), m[3], m[2], true, nil
}

// ── IDFC FIRST Bank Checking ─────────────────────────────────────────────────
// Spend:    "Spent Rs.473.00 from A/C XX6977 at ZEPTO MARKETPLACE PRIV on 09/04/26."
// Interest: "Monthly interest of INR.318.00 earned on your Savings A/c XX6977 ... on 31/05/26."

var idfcSpendRe = regexp.MustCompile(
	`(?i)Spent\s+Rs\.?([\d,]+(?:\.\d{1,2})?)\s+from\s+A/C\s+\S+\s+at\s+(.+?)\s+on\s+(\d{2}/\d{2}/\d{2})`)
var idfcInterestRe = regexp.MustCompile(
	`(?i)INR\.?([\d,]+(?:\.\d{1,2})?)\s+earned.*?(\d{2}/\d{2}/\d{2})`)

func ExtractIDFCChecking(sms string) (merchant, rawDate, rawAmt string, isDebit bool, err error) {
	if m := idfcSpendRe.FindStringSubmatch(sms); m != nil {
		log.Debugf("idfc_checking: spend match — rawAmt=%q rawDate=%q merchant=%q", m[1], m[3], strings.TrimSpace(m[2]))
		return strings.TrimSpace(m[2]), m[3], m[1], true, nil
	}
	if m := idfcInterestRe.FindStringSubmatch(sms); m != nil {
		log.Debugf("idfc_checking: interest match — rawAmt=%q rawDate=%q", m[1], m[2])
		return "Monthly interest", m[2], m[1], false, nil
	}
	log.Debugf("idfc_checking: no match — patterns expect 'Spent Rs.<amt> from A/C ... at <merchant> on DD/MM/YY' or interest credit")
	return "", "", "", false, fmt.Errorf("idfc_checking: no match")
}
