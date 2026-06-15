export function tierOf(confidence: number): "hot" | "warm" | "cold" | "archive" {
  if (confidence > 0.85) return "hot";
  if (confidence > 0.7) return "warm";
  if (confidence > 0.4) return "cold";
  return "archive";
}

export function initials(name: string): string {
  const parts = name.trim().split(/\s+/);
  if (parts.length === 1) return (parts[0][0] || "?").toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

export function relativeTime(iso?: string): string {
  if (!iso) return "—";
  const then = new Date(iso).getTime();
  const diff = Date.now() - then;
  const m = Math.round(diff / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${m}m ago`;
  const h = Math.round(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.round(h / 24);
  if (d < 30) return `${d}d ago`;
  return new Date(iso).toLocaleDateString();
}

export function dateTime(iso?: string): string {
  return iso ? new Date(iso).toLocaleString() : "—";
}

// Crypto-shredded content is AES-GCM ciphertext under a destroyed key.
export function isShredded(content?: string): boolean {
  return !!content && content.startsWith("enc:v1:");
}
export function displayContent(content?: string): string {
  if (isShredded(content)) return "🔒 [crypto-shredded — unrecoverable]";
  return content ?? "";
}
