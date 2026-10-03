package service

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/smtp"
	"strings"

	"topupku/model"
)

type EmailService struct {
	Host     string
	Port     string
	User     string
	Pass     string
	From     string
	FromName string
	BaseURL  string
}

func NewEmailService(host, port, user, pass, from, fromName, baseURL string) *EmailService {
	if from == "" && user != "" {
		from = user
	}
	if fromName == "" {
		fromName = "Azeotopup"
	}
	return &EmailService{
		Host:     host,
		Port:     port,
		User:     user,
		Pass:     pass,
		From:     from,
		FromName: fromName,
		BaseURL:  baseURL,
	}
}

func (e *EmailService) IsConfigured() bool {
	return e.Host != "" && e.Port != "" && e.User != "" && e.Pass != ""
}

func (e *EmailService) SendOrderReceipt(order *model.Order) error {
	if order.CustomerEmail == "" {
		return nil
	}

	subject := fmt.Sprintf("Bukti Pembelian Top-Up Sukses — #%s", order.ID)
	body := e.buildHTMLReceipt(order)

	if !e.IsConfigured() {
		log.Printf("[Email Notifier (SIMULASI)] Struk dikirim ke %s | Order: %s | Game: %s | SN: %s | Total: Rp %d",
			order.CustomerEmail, order.ID, order.GameName, order.DigiflazzSN, order.Price)
		return nil
	}

	// Prepare MIME message
	headers := make(map[string]string)
	headers["From"] = fmt.Sprintf("%s <%s>", e.FromName, e.From)
	headers["To"] = order.CustomerEmail
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=\"UTF-8\""

	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + body

	addr := fmt.Sprintf("%s:%s", e.Host, e.Port)
	auth := smtp.PlainAuth("", e.User, e.Pass, e.Host)

	// If port 465, use TLS directly
	if e.Port == "465" {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         e.Host,
		}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			log.Printf("[Email Error] TLS Dial to %s failed: %v", addr, err)
			return err
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, e.Host)
		if err != nil {
			log.Printf("[Email Error] SMTP client creation failed: %v", err)
			return err
		}
		defer client.Close()

		if err = client.Auth(auth); err != nil {
			log.Printf("[Email Error] Auth failed: %v", err)
			return err
		}
		if err = client.Mail(e.From); err != nil {
			return err
		}
		if err = client.Rcpt(order.CustomerEmail); err != nil {
			return err
		}
		w, err := client.Data()
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(message))
		if err != nil {
			return err
		}
		_ = w.Close()
		_ = client.Quit()
		log.Printf("[Email Sent] Berhasil mengirim struk ke %s untuk pesanan %s", order.CustomerEmail, order.ID)
		return nil
	}

	// Standard STARTTLS (e.g. port 587) or plain
	err := smtp.SendMail(addr, auth, e.From, []string{order.CustomerEmail}, []byte(message))
	if err != nil {
		log.Printf("[Email Error] Gagal mengirim email ke %s: %v", order.CustomerEmail, err)
		return err
	}

	log.Printf("[Email Sent] Berhasil mengirim bukti pembelian ke %s untuk pesanan %s", order.CustomerEmail, order.ID)
	return nil
}

func (e *EmailService) buildHTMLReceipt(order *model.Order) string {
	snDisplay := order.DigiflazzSN
	if strings.TrimSpace(snDisplay) == "" {
		snDisplay = "SUKSES-TERPROSES"
	}

	dateStr := order.CreatedAt.Format("02 Jan 2006, 15:04 WIB")
	orderURL := fmt.Sprintf("%s/order/%s", e.BaseURL, order.ID)

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Bukti Pembelian — Azeotopup</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; background-color: #F8FAFC; margin: 0; padding: 20px; color: #1E293B; }
    .card { max-width: 540px; margin: 0 auto; background: #FFFFFF; border-radius: 12px; border: 1px solid #E2E8F0; overflow: hidden; box-shadow: 0 4px 6px -1px rgba(0,0,0,0.05); }
    .header { background: #0F172A; padding: 24px; text-align: center; color: #FFFFFF; }
    .header h1 { margin: 0 0 4px; font-size: 20px; font-weight: 800; letter-spacing: -0.02em; }
    .header p { margin: 0; font-size: 13px; color: #94A3B8; }
    .content { padding: 24px; }
    .badge { display: inline-block; background: #ECFDF5; color: #047857; font-weight: 700; font-size: 12px; padding: 4px 12px; border-radius: 9999px; margin-bottom: 16px; border: 1px solid #A7F3D0; }
    .sn-box { background: #F1F5F9; border-radius: 8px; padding: 14px; margin-bottom: 20px; border-left: 4px solid #4F46E5; }
    .sn-label { font-size: 11px; text-transform: uppercase; color: #64748B; font-weight: 700; margin-bottom: 4px; }
    .sn-value { font-family: 'Courier New', Courier, monospace; font-size: 15px; font-weight: 700; color: #0F172A; word-break: break-all; }
    table { width: 100%%; border-collapse: collapse; margin-bottom: 20px; }
    td { padding: 10px 0; border-bottom: 1px solid #F1F5F9; font-size: 14px; }
    td:last-child { text-align: right; font-weight: 600; color: #0F172A; }
    .footer { text-align: center; padding: 18px 24px; font-size: 12px; color: #94A3B8; background: #F8FAFC; border-top: 1px solid #E2E8F0; }
    .btn { display: inline-block; background: #4F46E5; color: #FFFFFF !important; text-decoration: none; font-size: 13px; font-weight: 600; padding: 10px 20px; border-radius: 6px; margin-top: 10px; }
  </style>
</head>
<body>
  <div class="card">
    <div class="header">
      <h1>Azeotopup</h1>
      <p>Struk Resmi Pembelian Top-Up Game</p>
    </div>
    <div class="content">
      <div style="text-align: center;">
        <span class="badge">TRANSAKSI SUKSES</span>
      </div>

      <div class="sn-box">
        <div class="sn-label">Serial Number (SN) / Bukti Server:</div>
        <div class="sn-value">%s</div>
      </div>

      <table>
        <tr>
          <td style="color: #64748B;">Nomor Pesanan</td>
          <td>%s</td>
        </tr>
        <tr>
          <td style="color: #64748B;">Game</td>
          <td>%s</td>
        </tr>
        <tr>
          <td style="color: #64748B;">Akun Tujuan</td>
          <td>%s</td>
        </tr>
        <tr>
          <td style="color: #64748B;">Item / Produk</td>
          <td>%s</td>
        </tr>
        <tr>
          <td style="color: #64748B;">Waktu Pembelian</td>
          <td>%s</td>
        </tr>
        <tr>
          <td style="color: #64748B; font-weight: 700;">Total Pembayaran</td>
          <td style="color: #4F46E5; font-size: 16px; font-weight: 800;">Rp %d</td>
        </tr>
      </table>

      <div style="text-align: center; margin-top: 20px;">
        <a href="%s" class="btn">Lihat Status Pesanan di Web &rarr;</a>
      </div>
    </div>
    <div class="footer">
      Terima kasih telah berbelanja di Azeotopup. Simpan email ini sebagai bukti sah transaksi Anda.
    </div>
  </div>
</body>
</html>`,
		snDisplay,
		order.ID,
		order.GameName,
		order.FullCustomerNo(),
		order.ProductName,
		dateStr,
		order.Price,
		orderURL,
	)
}
