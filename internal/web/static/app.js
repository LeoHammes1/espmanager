// ESPManager UI glue: SSE-driven live refresh, drawer, toasts, copy, misc.
// HTMX (loaded before this file) does the fetching/swapping; this only bridges
// the SSE stream to [data-live] regions and wires the floating layers.

(function () {
  "use strict";

  function refreshLiveRegions() {
    document.querySelectorAll("[data-live]").forEach(function (el) {
      if (window.htmx) window.htmx.trigger(el, "refresh");
    });
  }

  function setLive(state) {
    document.querySelectorAll(".livedot").forEach(function (d) {
      d.dataset.state = state;
      var label = d.querySelector("[data-live-label]");
      if (label) label.textContent = state === "open" ? "Live" : "Reconnecting";
    });
  }

  function connectSSE() {
    if (!window.EventSource) return;
    if (!document.querySelector("[data-live]")) return; // no live regions (e.g. login page)
    var es = new EventSource("/events");
    var timer = null;
    es.onopen = function () {
      setLive("open");
    };
    es.onerror = function () {
      setLive("connecting");
    };
    es.onmessage = function () {
      if (timer) return; // debounce bursts of MQTT heartbeats
      timer = setTimeout(function () {
        timer = null;
        refreshLiveRegions();
      }, 300);
    };
  }

  function openDrawer() {
    document.body.classList.add("drawer-open");
    var d = document.getElementById("drawer");
    if (d) {
      var focusable = d.querySelector("input, select, button, a");
      if (focusable) focusable.focus();
    }
  }
  function closeDrawer() {
    document.body.classList.remove("drawer-open");
    var body = document.getElementById("drawer-body");
    if (body) body.innerHTML = "";
  }
  function closeSidebar() {
    document.body.classList.remove("sidebar-open");
  }

  function announce(text) {
    var live = document.getElementById("sr-announce");
    if (live) live.textContent = text;
  }

  function flash(btn, text) {
    if (!btn.dataset.label) btn.dataset.label = btn.textContent;
    announce(text);
    btn.textContent = text;
    setTimeout(function () {
      btn.textContent = btn.dataset.label;
    }, 1400);
  }

  // navigator.clipboard is undefined on insecure (plain-HTTP) origins, the common
  // homelab case, so fall back to execCommand and always tell the user the result —
  // the value is also selectable text next to the button.
  function copyValue(btn, value) {
    if (!value) return;
    function fallback() {
      var ok = false;
      try {
        var ta = document.createElement("textarea");
        ta.value = value;
        ta.style.position = "fixed";
        ta.style.opacity = "0";
        document.body.appendChild(ta);
        ta.select();
        ok = document.execCommand("copy");
        document.body.removeChild(ta);
      } catch (e) {
        ok = false;
      }
      flash(btn, ok ? "Copied" : "Press Ctrl+C");
    }
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(value).then(function () {
        flash(btn, "Copied");
      }).catch(fallback);
    } else {
      fallback();
    }
  }

  document.addEventListener("click", function (e) {
    var t = e.target;
    if (t.closest("[data-drawer-close]")) {
      closeDrawer();
      return;
    }
    if (t.closest("[data-sidebar-toggle]")) {
      document.body.classList.toggle("sidebar-open");
      return;
    }
    if (t.id === "scrim") {
      closeDrawer();
      closeSidebar();
      return;
    }
    var copy = t.closest("[data-copy]");
    if (copy) {
      copyValue(copy, copy.getAttribute("data-copy") || "");
      return;
    }
    var pw = t.closest("[data-toggle-pw]");
    if (pw) {
      var input = document.getElementById(pw.getAttribute("data-toggle-pw"));
      if (input) {
        var show = input.type === "password";
        input.type = show ? "text" : "password";
        pw.setAttribute("aria-pressed", String(show));
        pw.setAttribute("aria-label", show ? "Hide password" : "Show password");
        pw.textContent = show ? "Hide" : "Show";
      }
      return;
    }
  });

  document.addEventListener("keydown", function (e) {
    if (e.key === "Escape") {
      closeDrawer();
      closeSidebar();
    }
  });

  document.body.addEventListener("htmx:afterSwap", function (e) {
    if (e.target && e.target.id === "drawer-body") openDrawer();
  });

  document.body.addEventListener("toast", function (e) {
    var detail = e.detail || {};
    var region = document.getElementById("toasts");
    if (!region) return;
    var el = document.createElement("div");
    el.className = "toast";
    el.setAttribute("role", "status");
    el.dataset.variant = detail.variant || "ok";
    var span = document.createElement("span");
    span.textContent = detail.message || "";
    var close = document.createElement("button");
    close.className = "btn ghost close";
    close.type = "button";
    close.setAttribute("aria-label", "Dismiss");
    close.textContent = "×";
    close.addEventListener("click", function () {
      el.remove();
    });
    el.appendChild(span);
    el.appendChild(close);
    region.appendChild(el);
    if (el.dataset.variant !== "error") {
      setTimeout(function () {
        el.remove();
      }, 4000);
    }
  });

  connectSSE();
})();
