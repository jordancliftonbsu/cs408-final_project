/*
 * Shared layout loader.
 *
 * Fetches the shared header and footer partials and injects them into any
 * page that includes the placeholder elements:
 *
 *     <div data-include="header"></div>   ... page content ...   <div data-include="footer"></div>
 *
 * Every HTML page MUST load this script (defer) and include the two
 * placeholders so the site shares one header and one footer. Do not paste
 * nav/footer markup directly into a page — edit the partials instead.
 *
 * After the header is injected, this script checks the current session via
 * /api/auth/me and updates the navbar's auth slot (#nav-auth): logged-out
 * users see "Log In"; logged-in users see a profile link + "Log out".
 *
 * It also exposes a small shared helper on window.FL:
 *   - FL.getCurrentUser(): Promise<user|null>  (cached per page load)
 *   - FL.logout(): Promise<void>               (clears session, reloads)
 */
(function () {
  const PARTIALS = {
    header: "/partials/header.html",
    footer: "/partials/footer.html",
  };

  // Cache the /api/auth/me result so multiple callers on one page (navbar,
  // My Leagues panel, etc.) don't each hit the endpoint.
  let currentUserPromise = null;

  function getCurrentUser() {
    if (!currentUserPromise) {
      currentUserPromise = fetch("/api/auth/me", { headers: { Accept: "application/json" } })
        .then((res) => (res.ok ? res.json() : null))
        .catch(() => null);
    }
    return currentUserPromise;
  }

  async function logout() {
    try {
      await fetch("/api/auth/logout", { method: "POST" });
    } catch (err) {
      console.error("[layout] logout request failed:", err);
    }
    // Reload so every session-aware part of the page resets to logged-out.
    window.location.href = "/";
  }

  // Expose shared helpers for page scripts.
  window.FL = Object.assign(window.FL || {}, { getCurrentUser, logout });

  async function inject(el) {
    const name = el.getAttribute("data-include");
    const url = PARTIALS[name];
    if (!url) return;
    try {
      const res = await fetch(url);
      if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
      el.innerHTML = await res.text();
    } catch (err) {
      console.error(`[layout] failed to load "${name}" partial:`, err);
    }
  }

  function fillYear() {
    document
      .querySelectorAll("[data-current-year]")
      .forEach((el) => (el.textContent = new Date().getFullYear()));
  }

  // Escape user-provided text before inserting into innerHTML.
  function esc(str) {
    return String(str).replace(/[&<>"']/g, (c) => ({
      "&": "&amp;",
      "<": "&lt;",
      ">": "&gt;",
      '"': "&quot;",
      "'": "&#39;",
    }[c]));
  }

  // Swap the navbar auth slot based on the current session.
  async function updateNavAuth() {
    const slot = document.getElementById("nav-auth");
    if (!slot) return;

    const user = await getCurrentUser();
    if (!user) {
      // Logged out — keep the default "Log In" button (already in the markup).
      return;
    }

    // Logged in — show a profile link and a logout button.
    slot.classList.add("dropdown");
    slot.innerHTML = `
      <a class="btn btn-outline-accent btn-sm dropdown-toggle" href="#" role="button"
         data-bs-toggle="dropdown" aria-expanded="false">
        ${esc(user.username)}
      </a>
      <ul class="dropdown-menu dropdown-menu-end dropdown-dark">
        <li><a class="dropdown-item" href="/my-leagues">My Leagues</a></li>
        <li><hr class="dropdown-divider"></li>
        <li><button class="dropdown-item" type="button" id="nav-logout">Log out</button></li>
      </ul>`;

    const logoutBtn = document.getElementById("nav-logout");
    if (logoutBtn) logoutBtn.addEventListener("click", logout);
  }

  document.addEventListener("DOMContentLoaded", async () => {
    const includes = document.querySelectorAll("[data-include]");
    await Promise.all(Array.from(includes).map(inject));
    // Run after partials are injected so these can see the header/footer DOM.
    fillYear();
    await updateNavAuth();
  });
})();
