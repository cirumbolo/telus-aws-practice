// API_BASE is the single frontend config seam. Empty means same-origin (local,
// where the Go server also serves this page). For the S3 deployment, set it to
// the EC2 API URL, e.g. "https://api.example.com".
const API_BASE = "";

const pad = document.getElementById("pad");
const slug = location.pathname.slice(1);
document.title = slug || "note";

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

load();
