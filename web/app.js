// API_BASE is the single frontend config seam. Empty means same-origin (local,
// where the Go server also serves this page). For the S3 deployment, set it to
// the EC2 API URL, e.g. "https://api.example.com".
const API_BASE = "";

const pad = document.getElementById("pad");
const header = document.getElementById("slug-header");
let slug = location.pathname.slice(1);
document.title = slug || "note";
header.textContent = slug;

// Mirrors the server's slug validation (handler.go's slugRe) so an obviously
// bad rename is rejected client-side before hitting the network.
const SLUG_RE = /^[a-zA-Z0-9_-]{1,64}$/;

// Load the note on open. A missing note comes back as {"text": ""}, so a fresh
// pad just starts empty.
async function load() {
  try {
    const res = await fetch(`${API_BASE}/notes/${slug}`);
    if (!res.ok) return;
    const data = await res.json();
    pad.value = data.text || "";
  } catch (_) {
    // Network/parse error: leave the textarea empty.
  }
}

async function save() {
  try {
    await fetch(`${API_BASE}/notes/${slug}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ text: pad.value }),
    });
  } catch (_) {
    // Swallow; the next debounced save will retry the current contents.
  }
}

// Debounced auto-save: fire ~800 ms after the user stops typing so a PUT
// doesn't go out per keystroke.
let timer;
pad.addEventListener("input", () => {
  clearTimeout(timer);
  timer = setTimeout(save, 800);
});

// Renaming moves the note server-side and navigates the browser to the new
// URL, keeping the header and the address bar in sync. Fires on blur or
// Enter rather than per-keystroke, since a rename is a bigger operation
// than a save.
async function rename() {
  const newSlug = header.textContent.trim();
  if (newSlug === slug) return;
  if (!SLUG_RE.test(newSlug)) {
    header.textContent = slug;
    return;
  }
  try {
    const res = await fetch(`${API_BASE}/notes/${slug}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ slug: newSlug }),
    });
    if (!res.ok) {
      // Conflict (409), not found (404), etc. — revert and stay put.
      header.textContent = slug;
      return;
    }
    location.pathname = "/" + newSlug;
  } catch (_) {
    header.textContent = slug;
  }
}

header.addEventListener("blur", rename);
header.addEventListener("keydown", (e) => {
  if (e.key === "Enter") {
    e.preventDefault();
    header.blur();
  }
});

// Summary: flush any pending edit first (the server summarizes the saved
// note), then ask the API, which proxies to FuelIX so no key lives in this file.
const summaryBtn = document.getElementById("summary-btn");
const summaryBox = document.getElementById("summary");
const summaryText = document.getElementById("summary-text");

function showSummary(text, isError) {
  summaryText.textContent = text;
  summaryBox.classList.toggle("error", isError);
  summaryBox.hidden = false;
}

async function summarize() {
  summaryBtn.disabled = true;
  showSummary("Summarizing…", false);
  try {
    clearTimeout(timer);
    await save();
    const res = await fetch(`${API_BASE}/notes/${slug}/summary`, { method: "POST" });
    if (!res.ok) {
      const msg = res.status === 400 ? "Nothing to summarize yet." : "Couldn't generate a summary.";
      showSummary(msg, true);
      return;
    }
    const data = await res.json();
    showSummary(data.summary, false);
  } catch (_) {
    showSummary("Couldn't generate a summary.", true);
  } finally {
    summaryBtn.disabled = false;
  }
}

summaryBtn.addEventListener("click", summarize);

load();
