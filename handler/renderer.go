package handler

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var templateFuncs = template.FuncMap{
	"formatRupiah": func(val interface{}) string {
		var amount int64
		switch v := val.(type) {
		case int:
			amount = int64(v)
		case int64:
			amount = v
		case int32:
			amount = int64(v)
		case float64:
			amount = int64(v)
		default:
			amount = 0
		}
		isNeg := amount < 0
		if isNeg {
			amount = -amount
		}
		s := strconv.FormatInt(amount, 10)
		n := len(s)
		res := s
		if n > 3 {
			var parts []string
			remainder := n % 3
			if remainder > 0 {
				parts = append(parts, s[:remainder])
			}
			for i := remainder; i < n; i += 3 {
				parts = append(parts, s[i:i+3])
			}
			res = strings.Join(parts, ".")
		}
		if isNeg {
			return "-Rp " + res
		}
		return "Rp " + res
	},
	"formatNumber": func(val interface{}) string {
		var amount int64
		switch v := val.(type) {
		case int:
			amount = int64(v)
		case int64:
			amount = v
		case int32:
			amount = int64(v)
		case float64:
			amount = int64(v)
		default:
			amount = 0
		}
		isNeg := amount < 0
		if isNeg {
			amount = -amount
		}
		s := strconv.FormatInt(amount, 10)
		n := len(s)
		res := s
		if n > 3 {
			var parts []string
			remainder := n % 3
			if remainder > 0 {
				parts = append(parts, s[:remainder])
			}
			for i := remainder; i < n; i += 3 {
				parts = append(parts, s[i:i+3])
			}
			res = strings.Join(parts, ".")
		}
		if isNeg {
			return "-" + res
		}
		return res
	},
	"formatAZcoin": func(val interface{}) string {
		var amount int64
		switch v := val.(type) {
		case int:
			amount = int64(v)
		case int64:
			amount = v
		case int32:
			amount = int64(v)
		case float64:
			amount = int64(v)
		default:
			amount = 0
		}
		s := strconv.FormatInt(amount, 10)
		n := len(s)
		res := s
		if n > 3 {
			var parts []string
			remainder := n % 3
			if remainder > 0 {
				parts = append(parts, s[:remainder])
			}
			for i := remainder; i < n; i += 3 {
				parts = append(parts, s[i:i+3])
			}
			res = strings.Join(parts, ".")
		}
		return res + " AZcoin"
	},
	"azcoinPrice": func(price, cost, discountPercent int) int {
		return (price - (price*discountPercent)/100)
	},
	"formatDate": func(t time.Time) string {
		if t.IsZero() {
			return "-"
		}
		return t.Format("02 Jan 2006")
	},
	"formatDateTime": func(t time.Time) string {
		if t.IsZero() {
			return "-"
		}
		return t.Format("02 Jan 2006, 15:04 WIB")
	},
	"formatDateTimePtr": func(t *time.Time) string {
		if t == nil || t.IsZero() {
			return "-"
		}
		return t.Format("02 Jan 2006, 15:04 WIB")
	},
	"statusBadgeClass": func(status string) string {
		switch status {
		case "success":
			return "badge-success"
		case "paid":
			return "badge-info"
		case "processing":
			return "badge-warning"
		case "pending_payment":
			return "badge-pending"
		case "failed":
			return "badge-danger"
		case "expired":
			return "badge-muted"
		case "refund":
			return "badge-danger"
		default:
			return "badge-default"
		}
	},
	"dict": func(values ...interface{}) (map[string]interface{}, error) {
		if len(values)%2 != 0 {
			return nil, fmt.Errorf("dict requires even number of arguments")
		}
		dict := make(map[string]interface{}, len(values)/2)
		for i := 0; i < len(values); i += 2 {
			key, ok := values[i].(string)
			if !ok {
				return nil, fmt.Errorf("dict keys must be strings")
			}
			dict[key] = values[i+1]
		}
		return dict, nil
	},
	"add": func(a, b interface{}) int64 {
		toInt64 := func(v interface{}) int64 {
			switch n := v.(type) {
			case int:
				return int64(n)
			case int64:
				return n
			case int32:
				return int64(n)
			case float64:
				return int64(n)
			default:
				return 0
			}
		}
		return toInt64(a) + toInt64(b)
	},
	"sub": func(a, b interface{}) int64 {
		toInt64 := func(v interface{}) int64 {
			switch n := v.(type) {
			case int:
				return int64(n)
			case int64:
				return n
			case int32:
				return int64(n)
			case float64:
				return int64(n)
			default:
				return 0
			}
		}
		return toInt64(a) - toInt64(b)
	},
	"safeHTML": func(s string) template.HTML {
		return template.HTML(s)
	},
	"percentRemain": func(remaining, total int) int {
		if total <= 0 {
			return 100
		}
		pct := (remaining * 100) / total
		if pct > 100 {
			return 100
		}
		if pct < 0 {
			return 0
		}
		return pct
	},
	"percentSold": func(remaining, total int) int {
		if total <= 0 {
			return 100
		}
		sold := total - remaining
		if sold < 0 {
			sold = 0
		}
		pct := (sold * 100) / total
		if pct > 100 {
			return 100
		}
		if pct < 0 {
			return 0
		}
		return pct
	},
}

func RenderTemplate(w http.ResponseWriter, layoutFile string, pageFiles []string, data interface{}) {
	files := append([]string{layoutFile}, pageFiles...)
	tmpl, err := template.New("").Funcs(templateFuncs).ParseFiles(files...)
	if err != nil {
		http.Error(w, fmt.Sprintf("Template parse error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, fmt.Sprintf("Template execute error: %v", err), http.StatusInternalServerError)
	}
}
