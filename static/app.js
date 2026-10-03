// TopupKu / Azeotopup Frontend Interactions (v3.0 Mobile-Optimized)

// Global Helper: Copy to Clipboard with Mobile Toast
window.copyToClipboard = function (text, message) {
  if (!text) return;
  const showToast = (msg) => {
    let toast = document.getElementById("global_mobile_toast");
    if (!toast) {
      toast = document.createElement("div");
      toast.id = "global_mobile_toast";
      toast.className = "mobile-toast";
      document.body.appendChild(toast);
    }
    toast.textContent = msg || "Berhasil disalin ke clipboard!";
    toast.classList.add("show");
    setTimeout(() => {
      toast.classList.remove("show");
    }, 2500);
  };

  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(
      () => showToast(message),
      () => fallbackCopy(text, message)
    );
  } else {
    fallbackCopy(text, message);
  }

  function fallbackCopy(val, msg) {
    const tempInput = document.createElement("textarea");
    tempInput.value = val;
    tempInput.style.position = "fixed";
    tempInput.style.left = "-9999px";
    document.body.appendChild(tempInput);
    tempInput.select();
    try {
      document.execCommand("copy");
      showToast(msg);
    } catch (e) {
      alert("Gagal menyalin: " + val);
    }
    document.body.removeChild(tempInput);
  }
};

// Global Helper: Order Tracking Prompt
window.trackOrderPrompt = function () {
  const orderId = prompt("Masukkan ID Pesanan Anda (contoh: TK-2026...):");
  if (orderId && orderId.trim()) {
    window.location.href = "/order/" + encodeURIComponent(orderId.trim());
  }
};

document.addEventListener("DOMContentLoaded", function () {
  // --- Game Page Elements ---
  const productCards = document.querySelectorAll(".product-card");
  const selectedProductInput = document.getElementById("selected_product_id");
  const summaryProduct = document.getElementById("summary_product_name");
  const summaryPrice = document.getElementById("summary_price");
  const summaryTotal = document.getElementById("summary_total");
  const btnCheckout = document.getElementById("btn_checkout");
  const orderForm = document.getElementById("order_form");
  const errorAlert = document.getElementById("form_error_alert");

  // Mobile Bottom Bar Elements
  const mobileCheckoutBar = document.getElementById("mobile_checkout_bar");
  const mobileSummaryProduct = document.getElementById("mobile_summary_product");
  const mobileSummaryTotal = document.getElementById("mobile_summary_total");
  const btnMobileCheckout = document.getElementById("btn_mobile_checkout");

  if (mobileCheckoutBar) {
    document.body.classList.add("has-mobile-bar");
  }

  // Handle Product Card Selection
  if (productCards.length > 0) {
    productCards.forEach((card) => {
      card.addEventListener("click", function () {
        const id = this.getAttribute("data-id");
        productCards.forEach((c) => {
          if (c.getAttribute("data-id") === id) {
            c.classList.add("selected");
          } else {
            c.classList.remove("selected");
          }
        });

        const name = this.getAttribute("data-name");
        const formattedPrice = this.getAttribute("data-formatted-price");

        const isPromo = this.getAttribute("data-is-promo") === "true";
        const summaryPromoRow = document.getElementById("summary_promo_row");
        if (summaryPromoRow) {
          summaryPromoRow.style.display = isPromo ? "flex" : "none";
        }

        if (selectedProductInput) selectedProductInput.value = id;

        // Desktop Summary
        if (summaryProduct) summaryProduct.textContent = name;
        if (summaryPrice) summaryPrice.textContent = formattedPrice;
        if (summaryTotal) summaryTotal.textContent = formattedPrice;
        if (btnCheckout) btnCheckout.disabled = false;

        // Mobile Summary
        if (mobileSummaryProduct) mobileSummaryProduct.textContent = name;
        if (mobileSummaryTotal) mobileSummaryTotal.textContent = formattedPrice;
        if (btnMobileCheckout) btnMobileCheckout.disabled = false;
      });
    });
  }

  // Mobile Checkout Button Click Event
  if (btnMobileCheckout && orderForm) {
    btnMobileCheckout.addEventListener("click", function () {
      const customerNo = document.getElementById("customer_no");
      if (!customerNo || !customerNo.value.trim()) {
        showError("Silakan masukkan Nomor ID Akun game Anda terlebih dahulu!");
        if (customerNo) {
          customerNo.scrollIntoView({ behavior: "smooth", block: "center" });
          customerNo.focus();
          customerNo.style.borderColor = "#E11D48";
          customerNo.style.boxShadow = "0 0 0 3px rgba(225, 29, 72, 0.2)";
          setTimeout(() => {
            customerNo.style.borderColor = "";
            customerNo.style.boxShadow = "";
          }, 2500);
        }
        return;
      }

      const customerEmail = document.getElementById("customer_email");
      if (!customerEmail || !customerEmail.value.trim()) {
        showError("Silakan masukkan alamat email Anda untuk pengiriman invoice!");
        if (customerEmail) {
          customerEmail.scrollIntoView({ behavior: "smooth", block: "center" });
          customerEmail.focus();
          customerEmail.style.borderColor = "#E11D48";
          customerEmail.style.boxShadow = "0 0 0 3px rgba(225, 29, 72, 0.2)";
          setTimeout(() => {
            customerEmail.style.borderColor = "";
            customerEmail.style.boxShadow = "";
          }, 2500);
        }
        return;
      }

      if (orderForm.requestSubmit) {
        orderForm.requestSubmit();
      } else {
        const submitEvent = new Event("submit", { cancelable: true });
        orderForm.dispatchEvent(submitEvent);
      }
    });
  }

  // --- Category Filter Tabs ---
  const categoryTabs = document.querySelectorAll(".category-tab");
  const groupSections = document.querySelectorAll(".product-group-section");
  if (categoryTabs.length > 0) {
    categoryTabs.forEach((tab) => {
      tab.addEventListener("click", function () {
        categoryTabs.forEach((t) => t.classList.remove("active"));
        this.classList.add("active");

        const targetCategory = this.getAttribute("data-category");
        groupSections.forEach((section) => {
          if (targetCategory === "all" || section.getAttribute("data-group") === targetCategory) {
            section.style.display = "";
          } else {
            section.style.display = "none";
          }
        });
      });
    });
  }

  // --- Order Form Submission ---
  if (orderForm) {
    orderForm.addEventListener("submit", async function (e) {
      e.preventDefault();

      const gameCode = document.getElementById("game_code").value;
      const customerNo = document.getElementById("customer_no").value.trim();
      const customerNo2Elem = document.getElementById("customer_no2");
      const customerNo2 = customerNo2Elem ? customerNo2Elem.value.trim() : "";
      const customerEmailElem = document.getElementById("customer_email");
      const customerEmail = customerEmailElem ? customerEmailElem.value.trim() : "";
      const productId = selectedProductInput ? parseInt(selectedProductInput.value, 10) : 0;

      if (!customerNo) {
        showError("Nomor ID akun harus diisi!");
        return;
      }
      if (!productId) {
        showError("Silakan pilih nominal produk terlebih dahulu!");
        return;
      }
      if (!customerEmail) {
        showError("Alamat email wajib diisi untuk pengiriman invoice!");
        if (customerEmailElem) {
          customerEmailElem.scrollIntoView({ behavior: "smooth", block: "center" });
          customerEmailElem.focus();
          customerEmailElem.style.borderColor = "#E11D48";
          customerEmailElem.style.boxShadow = "0 0 0 3px rgba(225, 29, 72, 0.2)";
          setTimeout(() => {
            customerEmailElem.style.borderColor = "";
            customerEmailElem.style.boxShadow = "";
          }, 2500);
        }
        return;
      }

      const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
      if (!emailRegex.test(customerEmail)) {
        showError("Format alamat email tidak valid (contoh: nama@email.com)!");
        if (customerEmailElem) {
          customerEmailElem.focus();
        }
        return;
      }

      const setLoading = (loading) => {
        if (btnCheckout) {
          btnCheckout.disabled = loading;
          btnCheckout.innerHTML = loading
            ? '<span class="spinner"></span> Membuat Pesanan...'
            : "Lanjutkan Pembayaran &rarr;";
        }
        if (btnMobileCheckout) {
          btnMobileCheckout.disabled = loading;
          btnMobileCheckout.innerHTML = loading
            ? '<span>Memproses...</span>'
            : '<span>Beli Sekarang &rarr;</span>';
        }
      };

      setLoading(true);

      try {
        const response = await fetch("/api/order", {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            game_code: gameCode,
            customer_no: customerNo,
            customer_no2: customerNo2,
            customer_email: customerEmail,
            product_id: productId,
          }),
        });

        const res = await response.json();
        if (res.success && res.data && res.data.redirect_url) {
          window.location.href = res.data.redirect_url;
        } else {
          showError(res.message || "Gagal membuat pesanan. Silakan coba lagi.");
          setLoading(false);
        }
      } catch (err) {
        showError("Terjadi kesalahan jaringan. Cek koneksi internet Anda.");
        setLoading(false);
      }
    });
  }

  function showError(msg) {
    if (errorAlert) {
      errorAlert.textContent = msg;
      errorAlert.style.display = "block";
      errorAlert.scrollIntoView({ behavior: "smooth", block: "center" });
    } else {
      alert(msg);
    }
  }

  // --- Homepage Live Search ---
  const searchInput = document.getElementById("game_search_input");
  const clearBtn = document.getElementById("game_search_clear");
  const gameGrid = document.getElementById("game_grid");
  const noGameFound = document.getElementById("no_game_found");

  if (searchInput && gameGrid) {
    const gameCards = gameGrid.querySelectorAll(".game-card");

    const doSearch = () => {
      const q = searchInput.value.trim().toLowerCase();
      let matchCount = 0;

      if (clearBtn) {
        clearBtn.style.display = q ? "flex" : "none";
      }

      gameCards.forEach((card) => {
        const name = (card.getAttribute("data-name") || "").toLowerCase();
        const code = (card.getAttribute("data-code") || "").toLowerCase();
        if (name.includes(q) || code.includes(q)) {
          card.style.display = "";
          matchCount++;
        } else {
          card.style.display = "none";
        }
      });

      if (noGameFound) {
        noGameFound.style.display = matchCount === 0 ? "block" : "none";
      }
    };

    searchInput.addEventListener("input", doSearch);

    if (clearBtn) {
      clearBtn.addEventListener("click", () => {
        searchInput.value = "";
        doSearch();
        searchInput.focus();
      });
    }
  }

  // --- Order Status Page Logic (Live Polling & Countdown) ---
  const orderStatusContainer = document.getElementById("order_status_container");
  if (orderStatusContainer) {
    const orderId = orderStatusContainer.getAttribute("data-order-id");
    let currentStatus = orderStatusContainer.getAttribute("data-status");
    const statusBadge = document.getElementById("live_status_badge");
    const snBox = document.getElementById("live_sn_container");
    const snText = document.getElementById("live_sn_text");
    const expiryTimestamp = parseInt(orderStatusContainer.getAttribute("data-expiry-unix"), 10);

    // Timer Countdown
    const countdownElem = document.getElementById("countdown_timer");
    if (countdownElem && expiryTimestamp && currentStatus === "pending_payment") {
      const updateTimer = () => {
        const now = Math.floor(Date.now() / 1000);
        const remaining = expiryTimestamp - now;

        if (remaining <= 0) {
          countdownElem.textContent = "00:00 (Kadaluarsa)";
          clearInterval(timerInterval);
          return;
        }

        const mins = Math.floor(remaining / 60);
        const secs = remaining % 60;
        countdownElem.textContent = `${String(mins).padStart(2, "0")}:${String(secs).padStart(2, "0")}`;
      };

      updateTimer();
      const timerInterval = setInterval(updateTimer, 1000);
    }

    // Polling every 3 seconds if status is pending_payment, paid, or processing
    if (["pending_payment", "paid", "processing"].includes(currentStatus)) {
      const pollInterval = setInterval(async () => {
        try {
          const res = await fetch(`/api/order/${orderId}/status`);
          const data = await res.json();

          if (data.success && data.data) {
            const newStatus = data.data.status;
            if (newStatus !== currentStatus) {
              currentStatus = newStatus;
              if (statusBadge) {
                statusBadge.textContent = data.data.status_text;
                statusBadge.className = "status-badge " + getBadgeClass(newStatus);
              }

              if (newStatus === "success" && snBox && snText) {
                snText.textContent = data.data.sn || "-";
                snBox.style.display = "block";
              }

              if (["success", "failed", "expired", "refund"].includes(newStatus)) {
                clearInterval(pollInterval);
                setTimeout(() => window.location.reload(), 1500);
              }
            }
          }
        } catch (e) {
          console.error("Polling error:", e);
        }
      }, 3000);
    }

    // Simulate Pay Handler (Dev Mode)
    const btnSimulatePay = document.getElementById("btn_simulate_pay");
    if (btnSimulatePay) {
      btnSimulatePay.addEventListener("click", async function () {
        this.disabled = true;
        this.textContent = "Memproses simulasi...";

        try {
          const res = await fetch(`/api/order/${orderId}/simulate-pay`, {
            method: "POST",
          });
          const result = await res.json();
          if (result.success) {
            setTimeout(() => window.location.reload(), 1000);
          } else {
            alert(result.message || "Simulasi gagal");
            this.disabled = false;
            this.textContent = "Simulasikan Pembayaran QRIS Berhasil";
          }
        } catch (err) {
          alert("Gagal menghubungi server");
          this.disabled = false;
        }
      });
    }
  }

  function getBadgeClass(status) {
    switch (status) {
      case "success": return "badge-success";
      case "paid": return "badge-info";
      case "processing": return "badge-warning";
      case "pending_payment": return "badge-pending";
      case "failed": return "badge-danger";
      case "expired": return "badge-muted";
      default: return "badge-default";
    }
  }
});
