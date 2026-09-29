// Chart-of-accounts parsing: turns the embedded chart-of-accounts.md
// tables into machine-usable account and class registries so journal
// preparation does not require an agent to eyeball a markdown table.
package policy

import (
	"fmt"
	"sort"
	"strings"
)

// Account is one row of the chart of accounts.
type Account struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Section string `json:"section"`
}

// QBClass is one row of the class inventory.
type QBClass struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// splitMDRow parses a "| a | b | c |" table row into trimmed cells.
func splitMDRow(line string) ([]string, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") {
		return nil, false
	}
	line = strings.TrimPrefix(line, "|")
	parts := strings.Split(line, "|")
	cells := make([]string, 0, len(parts))
	for _, p := range parts {
		cells = append(cells, strings.TrimSpace(p))
	}
	// Skip header/separator rows.
	for _, c := range cells {
		if c == "" {
			continue
		}
		if strings.HasPrefix(c, "---") || strings.EqualFold(c, "ID") || strings.EqualFold(c, "Fully qualified name") {
			return nil, false
		}
	}
	return cells, true
}

// parseCOA walks the doc once and collects accounts (sections other than the
// class inventory) and classes.
func parseCOA(company string) ([]Account, []QBClass, error) {
	content, err := Doc(company, "chart-of-accounts")
	if err != nil {
		return nil, nil, err
	}
	var accounts []Account
	var classes []QBClass
	section := ""
	inClasses := false
	for line := range strings.SplitSeq(content, "\n") {
		trim := strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(trim, "## "); ok {
			section = after
			// Strip the trailing "(N)" count.
			if i := strings.LastIndex(section, "("); i > 0 {
				section = strings.TrimSpace(section[:i])
			}
			inClasses = strings.HasPrefix(strings.ToLower(section), "class inventory")
			continue
		}
		cells, ok := splitMDRow(trim)
		if !ok || len(cells) < 2 {
			continue
		}
		if inClasses {
			classes = append(classes, QBClass{ID: cells[0], Name: cells[1]})
			continue
		}
		a := Account{ID: cells[0], Name: cells[1], Section: section}
		if len(cells) > 2 {
			a.Type = cells[2]
		}
		if len(cells) > 3 {
			a.Subtype = cells[3]
		}
		accounts = append(accounts, a)
	}
	return accounts, classes, nil
}

// Accounts returns the parsed chart of accounts for a company.
func Accounts(company string) ([]Account, error) {
	a, _, err := parseCOA(company)
	return a, err
}

// Classes returns the parsed class inventory for a company.
func Classes(company string) ([]QBClass, error) {
	_, c, err := parseCOA(company)
	return c, err
}

// QueryAccounts matches accounts by exact id or case-insensitive name
// substring.
func QueryAccounts(company, query string) ([]Account, error) {
	accounts, err := Accounts(company)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var out []Account
	for _, a := range accounts {
		if q == "" || a.ID == query || strings.Contains(strings.ToLower(a.Name), q) {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// QueryClasses matches classes by exact id or case-insensitive name
// substring.
func QueryClasses(company, query string) ([]QBClass, error) {
	classes, err := Classes(company)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var out []QBClass
	for _, c := range classes {
		if q == "" || c.ID == query || strings.Contains(strings.ToLower(c.Name), q) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// AccountNameForID resolves an account id to its name via the parsed COA;
// "" when the id is unknown. Used by the checker after the policy-map lookup.
func AccountNameForID(company, id string) string {
	accounts, err := Accounts(company)
	if err != nil {
		return ""
	}
	for _, a := range accounts {
		if a.ID == id {
			return a.Name
		}
	}
	return ""
}

// resolveCompanyForCOA is a small internal helper mirroring the CLI's
// single-profile default (avoids an import cycle through the cli package).
func resolveCompanyForCOA(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	companies, err := Companies()
	if err != nil {
		return "", err
	}
	if len(companies) == 1 {
		return companies[0], nil
	}
	return "", fmt.Errorf("multiple company profiles embedded; pass --company")
}
