package tenant

import (
	"context"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
)

// The standard reference data a new tenant starts with: the currencies (ISO 4217, with their minor units) and the
// countries (ISO 3166-1) most used in Indonesian trade. A tenant gets them once, while its table is still empty, and
// may then deactivate, change or add its own; a country deleted by the tenant does not come back.

type currencyRef struct {
	iso, name, symbol string
	decimals          int
}

var Currencies = []currencyRef{
	{"IDR", "Rupiah Indonesia", "Rp", 2}, {"USD", "Dolar Amerika Serikat", "$", 2}, {"EUR", "Euro", "€", 2},
	{"SGD", "Dolar Singapura", "S$", 2}, {"MYR", "Ringgit Malaysia", "RM", 2}, {"THB", "Baht Thailand", "฿", 2},
	{"VND", "Dong Vietnam", "₫", 0}, {"PHP", "Peso Filipina", "₱", 2}, {"BND", "Dolar Brunei", "B$", 2},
	{"KHR", "Riel Kamboja", "៛", 2}, {"MMK", "Kyat Myanmar", "K", 2}, {"LAK", "Kip Laos", "₭", 2},
	{"CNY", "Yuan Tiongkok", "¥", 2}, {"HKD", "Dolar Hong Kong", "HK$", 2}, {"TWD", "Dolar Taiwan Baru", "NT$", 2},
	{"JPY", "Yen Jepang", "¥", 0}, {"KRW", "Won Korea Selatan", "₩", 0}, {"INR", "Rupee India", "₹", 2},
	{"BDT", "Taka Bangladesh", "৳", 2}, {"PKR", "Rupee Pakistan", "Rs", 2}, {"LKR", "Rupee Sri Lanka", "Rs", 2},
	{"AUD", "Dolar Australia", "A$", 2}, {"NZD", "Dolar Selandia Baru", "NZ$", 2}, {"CAD", "Dolar Kanada", "C$", 2},
	{"MXN", "Peso Meksiko", "Mex$", 2}, {"BRL", "Real Brasil", "R$", 2}, {"GBP", "Pound Sterling", "£", 2},
	{"CHF", "Franc Swiss", "CHF", 2}, {"TRY", "Lira Turki", "₺", 2}, {"RUB", "Rubel Rusia", "₽", 2},
	{"AED", "Dirham Uni Emirat Arab", "AED", 2}, {"SAR", "Riyal Arab Saudi", "SAR", 2}, {"EGP", "Pound Mesir", "E£", 2},
	{"ZAR", "Rand Afrika Selatan", "R", 2}, {"NGN", "Naira Nigeria", "₦", 2}, {"KES", "Shilling Kenya", "KSh", 2},
}

type countryRef struct {
	iso2, iso3, phone, name, currency string
}

var Countries = []countryRef{
	{"ID", "IDN", "+62", "Indonesia", "IDR"}, {"SG", "SGP", "+65", "Singapura", "SGD"}, {"MY", "MYS", "+60", "Malaysia", "MYR"},
	{"TH", "THA", "+66", "Thailand", "THB"}, {"VN", "VNM", "+84", "Vietnam", "VND"}, {"PH", "PHL", "+63", "Filipina", "PHP"},
	{"BN", "BRN", "+673", "Brunei Darussalam", "BND"}, {"KH", "KHM", "+855", "Kamboja", "KHR"}, {"MM", "MMR", "+95", "Myanmar", "MMK"},
	{"LA", "LAO", "+856", "Laos", "LAK"}, {"TL", "TLS", "+670", "Timor Leste", "USD"}, {"CN", "CHN", "+86", "Tiongkok", "CNY"},
	{"HK", "HKG", "+852", "Hong Kong", "HKD"}, {"TW", "TWN", "+886", "Taiwan", "TWD"}, {"JP", "JPN", "+81", "Jepang", "JPY"},
	{"KR", "KOR", "+82", "Korea Selatan", "KRW"}, {"IN", "IND", "+91", "India", "INR"}, {"BD", "BGD", "+880", "Bangladesh", "BDT"},
	{"PK", "PAK", "+92", "Pakistan", "PKR"}, {"LK", "LKA", "+94", "Sri Lanka", "LKR"}, {"AU", "AUS", "+61", "Australia", "AUD"},
	{"NZ", "NZL", "+64", "Selandia Baru", "NZD"}, {"US", "USA", "+1", "Amerika Serikat", "USD"}, {"CA", "CAN", "+1", "Kanada", "CAD"},
	{"MX", "MEX", "+52", "Meksiko", "MXN"}, {"BR", "BRA", "+55", "Brasil", "BRL"}, {"GB", "GBR", "+44", "Britania Raya", "GBP"},
	{"NL", "NLD", "+31", "Belanda", "EUR"}, {"DE", "DEU", "+49", "Jerman", "EUR"}, {"FR", "FRA", "+33", "Prancis", "EUR"},
	{"IT", "ITA", "+39", "Italia", "EUR"}, {"ES", "ESP", "+34", "Spanyol", "EUR"}, {"BE", "BEL", "+32", "Belgia", "EUR"},
	{"CH", "CHE", "+41", "Swiss", "CHF"}, {"TR", "TUR", "+90", "Turki", "TRY"}, {"RU", "RUS", "+7", "Rusia", "RUB"},
	{"AE", "ARE", "+971", "Uni Emirat Arab", "AED"}, {"SA", "SAU", "+966", "Arab Saudi", "SAR"}, {"EG", "EGY", "+20", "Mesir", "EGP"},
	{"ZA", "ZAF", "+27", "Afrika Selatan", "ZAR"}, {"NG", "NGA", "+234", "Nigeria", "NGN"}, {"KE", "KEN", "+254", "Kenya", "KES"},
}

// seedReference adds the standard currencies and countries to a tenant whose tables are still empty (deleted rows
// count: what the tenant removed stays removed). ctx carries the tenant's connection.
func seedReference(ctx context.Context, db *database.DB) error {
	codes := codeprefix.Postgres{DB: db}
	empty := func(table string) (bool, error) {
		var any bool
		err := db.Q(ctx).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pm.`+table+`)`).Scan(&any)
		return !any, err
	}
	if ok, err := empty("currency"); err != nil {
		return err
	} else if ok {
		for _, c := range Currencies {
			code, err := codes.Next(ctx, "currency")
			if err != nil {
				return err
			}
			if _, err := db.Q(ctx).Exec(ctx, `INSERT INTO pm.currency (code, name, iso_code, symbol, decimals, created_by) VALUES ($1, $2, $3, $4, $5, $6)`,
				code, c.name, c.iso, c.symbol, c.decimals, systemUser); err != nil {
				return err
			}
		}
	}
	if ok, err := empty("country"); err != nil {
		return err
	} else if ok {
		for _, c := range Countries {
			code, err := codes.Next(ctx, "country")
			if err != nil {
				return err
			}
			if _, err := db.Q(ctx).Exec(ctx, `INSERT INTO pm.country (code, name, iso2, iso3, phone_code, currency_id, created_by)
				VALUES ($1, $2, $3, $4, $5, (SELECT id FROM pm.currency WHERE iso_code = $6 AND NOT is_deleted), $7)`,
				code, c.name, c.iso2, c.iso3, c.phone, c.currency, systemUser); err != nil {
				return err
			}
		}
	}
	return nil
}
