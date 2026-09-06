#!/usr/bin/env node
// Draws every brand asset from one path: the favicons, the web manifest, the
// logo for structured data, a share image for every page, the dashboard's
// icons, and the Android drawables. Run it after changing the mark, a page
// title, or a page description.
//
//   node scripts/brand.mjs            write everything
//   node scripts/brand.mjs --check    compare with what is committed; exit 1 on drift (CI)
//
// Text is set with satori from static instances of Instrument Sans and Geist
// Mono (the @fontsource packages in the root devDependencies), so the images
// come out the same on every machine; sharp turns the SVG into PNG.
// docs/decisions.md 019 has the geometry and the reasons.
import satori from "satori";
import sharp from "sharp";
import { existsSync, mkdirSync, readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const check = process.argv.includes("--check");
const drift = [];
const log = (m) => console.log(m);
const fail = (m) => {
  console.error(`brand: ${m}`);
  process.exit(1);
};

// 1. The mark: a SIM card on a 24-unit grid. One compound path; the chip's
//    pads are holes, so the same drawing works on any background, as an
//    alpha-only Android icon, and as a notification glyph.
const MARK = "M4.5 1.5H15L19.5 6V22.5H4.5ZM7.5 10.5V13.5H11.25V10.5ZM12.75 10.5V13.5H16.5V10.5ZM7.5 15V18H11.25V15ZM12.75 15V18H16.5V15Z";
// The same drawing at every size, 16 px included: the cross goes soft there,
// and that is accepted so the mark never changes shape.
const FULL = "0 0 24 24"; // favicons, notification icon
const FRAMED = "-3 -3 30 30"; // touch icon, manifest icons, logo: the mark box is 80 % of the canvas
const MASK = "-5 -5 34 34"; // maskable icon: the card stays inside the 80 % safe circle
const INK = "#111111";
const PAPER = "#ffffff";
const MUTED = "#6b6b68";
const LINE = "#e6e6e3";
const DEFAULT_DESCRIPTION =
  "Turn an Android phone into an SMS API. Send and receive texts from your code through your own SIM, with webhooks, an SDK, and an MCP server.";

function svgOf(d, { size, viewBox = FULL, fill = INK, background, darkMode = false } = {}) {
  const [x, y, w, h] = viewBox.split(" ");
  return (
    `<svg xmlns="http://www.w3.org/2000/svg"${size ? ` width="${size}" height="${size}"` : ""} viewBox="${viewBox}">` +
    (darkMode ? "<style>@media (prefers-color-scheme:dark){path{fill:#fff}}</style>" : "") +
    (background ? `<rect x="${x}" y="${y}" width="${w}" height="${h}" fill="${background}"/>` : "") +
    `<path fill="${fill}" fill-rule="evenodd" d="${d}"/></svg>`
  );
}

// 2. Raster. The SVG carries its pixel size, so librsvg draws at that size
//    and nothing is resampled afterwards.
async function png(svg, { opaque = false } = {}) {
  let img = sharp(Buffer.from(svg), { density: 72 });
  if (opaque) img = img.flatten({ background: PAPER }).removeAlpha();
  return img.png({ compressionLevel: 9, adaptiveFiltering: false, palette: false }).toBuffer();
}

// 3. ICO: a directory of PNG payloads. Every current browser and Windows read this form.
function ico(entries) {
  const header = Buffer.alloc(6 + 16 * entries.length);
  header.writeUInt16LE(0, 0);
  header.writeUInt16LE(1, 2);
  header.writeUInt16LE(entries.length, 4);
  let offset = header.length;
  entries.forEach(({ size, data }, i) => {
    const e = 6 + i * 16;
    header[e] = size === 256 ? 0 : size;
    header[e + 1] = size === 256 ? 0 : size;
    header[e + 2] = 0;
    header[e + 3] = 0;
    header.writeUInt16LE(1, e + 4);
    header.writeUInt16LE(32, e + 6);
    header.writeUInt32LE(data.length, e + 8);
    header.writeUInt32LE(offset, e + 12);
    offset += data.length;
  });
  return Buffer.concat([header, ...entries.map((e) => e.data)]);
}

function icoPayloads(buf) {
  const count = buf.readUInt16LE(4);
  const out = [];
  for (let i = 0; i < count; i++) {
    const e = 6 + i * 16;
    const length = buf.readUInt32LE(e + 8);
    const offset = buf.readUInt32LE(e + 12);
    out.push(buf.subarray(offset, offset + length));
  }
  return out;
}

// 4. Type. satori wants static instances (it draws variable fonts at their
//    default weight), in TTF, OTF, or WOFF.
const fontFile = (pkg, file) => {
  const p = path.join(root, "node_modules", "@fontsource", pkg, "files", file);
  if (!existsSync(p)) fail(`${p} is missing: run pnpm install`);
  return readFileSync(p);
};
const fonts = [
  { name: "Instrument Sans", data: fontFile("instrument-sans", "instrument-sans-latin-400-normal.woff"), weight: 400, style: "normal" },
  { name: "Instrument Sans", data: fontFile("instrument-sans", "instrument-sans-latin-600-normal.woff"), weight: 600, style: "normal" },
  { name: "Geist Mono", data: fontFile("geist-mono", "geist-mono-latin-400-normal.woff"), weight: 400, style: "normal" },
  { name: "Geist Mono", data: fontFile("geist-mono", "geist-mono-latin-500-normal.woff"), weight: 500, style: "normal" },
];
const h = (type, style, ...children) => ({ type, props: { style, children: children.length === 1 ? children[0] : children } });

// 5. The pages. Titles and descriptions of .astro pages live in
//    site/src/lib/pages.json; Markdown pages carry theirs in frontmatter.
const slugOf = (route) => (route === "/" ? "home" : route.slice(1).replace(/\//g, "-"));

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    const p = path.join(dir, name);
    if (statSync(p).isDirectory()) yield* walk(p);
    else yield p;
  }
}

function pages() {
  const out = [];
  const registry = JSON.parse(readFileSync(path.join(root, "site", "src", "lib", "pages.json"), "utf8"));
  for (const [route, entry] of Object.entries(registry)) {
    if (route === "/404") continue;
    out.push({ route, title: entry.og ?? entry.title, description: entry.description ?? DEFAULT_DESCRIPTION });
  }
  const dir = path.join(root, "site", "src", "pages");
  for (const file of walk(dir)) {
    if (!file.endsWith(".md")) continue;
    const rel = path.relative(dir, file).replace(/\\/g, "/");
    const route = "/" + rel.replace(/\.md$/, "").replace(/(^|\/)index$/, "");
    const fm = readFileSync(file, "utf8").match(/^---\r?\n([\s\S]*?)\r?\n---/)?.[1] ?? "";
    const get = (key) => {
      const m = fm.match(new RegExp(`^${key}:[ \\t]*(.+)$`, "m"));
      return m ? m[1].trim().replace(/^"(.*)"$/, "$1") : undefined;
    };
    const title = get("og") ?? get("headTitle") ?? get("title");
    if (!title) fail(`${rel}: no title in its frontmatter`);
    out.push({ route: route.replace(/\/$/, "") || "/", title, description: get("description") ?? DEFAULT_DESCRIPTION });
  }
  return out.map((p) => ({ ...p, slug: slugOf(p.route), eyebrow: p.route.startsWith("/docs") ? "docs" : undefined }));
}

// 6. Share images: white, the mark, the wordmark, the title, one muted line.
async function card({ W, H, title, description, eyebrow, url }) {
  const R = W - 80;
  if (title.length > 56) log(`  note: "${title}" is ${title.length} characters and may be clamped to two lines`);
  const svg = await satori(
    h(
      "div",
      { width: W, height: H, display: "flex", position: "relative", backgroundColor: PAPER, color: INK },
      h("div", { position: "absolute", left: 187, top: 94.5, height: 60, display: "flex", alignItems: "center", fontFamily: "Geist Mono", fontWeight: 500, fontSize: 44 }, "simhook"),
      eyebrow
        ? h("div", { position: "absolute", left: 80, top: 224, display: "flex", fontFamily: "Geist Mono", fontWeight: 400, fontSize: 24, lineHeight: 1.33, color: MUTED }, eyebrow)
        : h("div", { position: "absolute", left: 0, top: 0, width: 0, height: 0, display: "flex" }),
      h("div", { position: "absolute", left: 80, top: 268, width: R - 80, display: "block", fontFamily: "Instrument Sans", fontWeight: 600, fontSize: 72, lineHeight: 1.11, letterSpacing: -0.72, lineClamp: 2 }, title),
      h("div", { position: "absolute", left: 80, top: H - 150, width: R - 80, height: 1, display: "flex", backgroundColor: LINE }),
      // One row: the description takes what the address leaves, and is cut with an ellipsis rather than run under it.
      h(
        "div",
        { position: "absolute", left: 80, top: H - 118, width: R - 80, display: "flex", flexDirection: "row", justifyContent: "space-between", alignItems: "center", color: MUTED },
        h("div", { flexShrink: 1, minWidth: 0, display: "block", fontFamily: "Instrument Sans", fontWeight: 400, fontSize: 26, lineHeight: 1.46, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }, description),
        h("div", { flexShrink: 0, marginLeft: 40, display: "flex", fontFamily: "Geist Mono", fontWeight: 400, fontSize: 26, lineHeight: 1.46 }, url),
      ),
    ),
    { width: W, height: H, fonts },
  );
  // The mark is our own path, so nothing has to decode a nested image.
  const withMark = svg.replace(/<\/svg>\s*$/, `<path transform="translate(57.5 64.5) scale(5)" fill="${INK}" fill-rule="evenodd" d="${MARK}"/></svg>`);
  return png(withMark, { opaque: true });
}

// 7. Android drawables, from the same path. The card's circumradius is
//    12.9 units; at scale 2.4 that is 30.96 dp, inside the 33 dp safe circle
//    of the 108 dp adaptive canvas.
const androidPath = MARK.replace(/([MLHVZ])/g, " $1").trim();
const launcherForeground = `<?xml version="1.0" encoding="utf-8"?>
<!-- Generated by scripts/brand.mjs from the mark; run pnpm brand instead of editing.
     The chip's pads are holes, so the same drawable serves as the monochrome layer. -->
<vector xmlns:android="http://schemas.android.com/apk/res/android"
    android:width="108dp"
    android:height="108dp"
    android:viewportWidth="108"
    android:viewportHeight="108">
    <group
        android:scaleX="2.4"
        android:scaleY="2.4"
        android:translateX="25.2"
        android:translateY="25.2">
        <path
            android:fillColor="#FF111111"
            android:fillType="evenOdd"
            android:pathData="${androidPath}" />
    </group>
</vector>
`;
const notificationIcon = `<?xml version="1.0" encoding="utf-8"?>
<!-- Generated by scripts/brand.mjs from the mark; run pnpm brand instead of editing.
     Notification icons are drawn from their alpha alone, so this is the mark in white. -->
<vector xmlns:android="http://schemas.android.com/apk/res/android"
    android:width="24dp"
    android:height="24dp"
    android:viewportWidth="24"
    android:viewportHeight="24">
    <path
        android:fillColor="#FFFFFFFF"
        android:fillType="evenOdd"
        android:pathData="${androidPath}" />
</vector>
`;

const manifest = {
  name: "simhook",
  short_name: "simhook",
  description: "Turn an Android phone into an SMS API.",
  start_url: "/",
  display: "browser",
  background_color: PAPER,
  theme_color: PAPER,
  icons: [
    { src: "/icon-192.png", sizes: "192x192", type: "image/png" },
    { src: "/icon-512.png", sizes: "512x512", type: "image/png" },
    { src: "/icon-maskable-512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
  ],
};

// 8. Writing, or in --check mode comparing. Text is compared byte for byte;
//    rasters as pixels, because librsvg builds differ by a shade across platforms.
const raw = (buf) => sharp(buf).ensureAlpha().raw().toBuffer();
async function samePixels(a, b) {
  const [x, y] = await Promise.all([raw(a), raw(b)]);
  if (x.length !== y.length) return false;
  let bad = 0;
  for (let i = 0; i < x.length; i++) if (Math.abs(x[i] - y[i]) > 8) bad++;
  return bad <= x.length * 0.001;
}
async function emit(rel, content) {
  const bytes = Buffer.isBuffer(content) ? content : Buffer.from(content);
  const file = path.join(root, rel);
  if (!check) {
    mkdirSync(path.dirname(file), { recursive: true });
    writeFileSync(file, bytes);
    log(`wrote ${rel}`);
    return;
  }
  if (!existsSync(file)) return drift.push(`${rel}: missing`);
  const have = readFileSync(file);
  if (have.equals(bytes)) return;
  if (rel.endsWith(".png")) {
    if (!(await samePixels(bytes, have))) drift.push(`${rel}: pixels differ`);
  } else if (rel.endsWith(".ico")) {
    const a = icoPayloads(bytes);
    const b = icoPayloads(have);
    if (a.length !== b.length) return drift.push(`${rel}: entry count differs`);
    for (let i = 0; i < a.length; i++) if (!(await samePixels(a[i], b[i]))) return drift.push(`${rel}: entry ${i} differs`);
  } else {
    drift.push(`${rel}: differs`);
  }
}

async function main() {
  const site = "site/public";
  const favicon = svgOf(MARK, { darkMode: true });
  await emit(`${site}/favicon.svg`, favicon);
  await emit(`${site}/brand/mark.svg`, `<!-- The simhook mark. Original work on a 24-unit grid; docs/decisions.md 019. -->\n${svgOf(MARK)}\n`);
  await emit(
    `${site}/favicon.ico`,
    ico([
      { size: 16, data: await png(svgOf(MARK, { size: 16 })) },
      { size: 32, data: await png(svgOf(MARK, { size: 32 })) },
      { size: 48, data: await png(svgOf(MARK, { size: 48 })) },
    ]),
  );
  await emit(`${site}/favicon-96.png`, await png(svgOf(MARK, { size: 96 })));
  await emit(`${site}/apple-touch-icon.png`, await png(svgOf(MARK, { size: 180, viewBox: FRAMED, background: PAPER }), { opaque: true }));
  await emit(`${site}/icon-192.png`, await png(svgOf(MARK, { size: 192, viewBox: FRAMED })));
  await emit(`${site}/icon-512.png`, await png(svgOf(MARK, { size: 512, viewBox: FRAMED })));
  await emit(`${site}/icon-maskable-512.png`, await png(svgOf(MARK, { size: 512, viewBox: MASK, background: PAPER }), { opaque: true }));
  await emit(`${site}/brand/logo-512.png`, await png(svgOf(MARK, { size: 512, viewBox: FRAMED, background: PAPER }), { opaque: true }));
  await emit(`${site}/site.webmanifest`, `${JSON.stringify(manifest, null, 2)}\n`);

  const all = pages();
  log(`share images for ${all.length} pages`);
  for (const p of all) {
    await emit(`${site}/og/${p.slug}.png`, await card({ W: 1200, H: 630, title: p.title, description: p.description, eyebrow: p.eyebrow, url: "simhook.dev" }));
  }
  const home = all.find((p) => p.route === "/");
  if (!home) fail("pages.json has no entry for /");
  const homeCard = await card({ W: 1200, H: 630, title: home.title, description: home.description, url: "simhook.dev" });
  await emit(`${site}/og/default.png`, homeCard);
  await emit(
    `${site}/brand/github-social-1280x640.png`,
    await card({
      W: 1280,
      H: 640,
      title: "simhook turns an Android phone into an SMS API.",
      description: "Go API, Android app, dashboard, SDK, MCP server.",
      url: "github.com/simhook/simhook",
    }),
  );

  const web = "web/src/app";
  await emit(`${web}/icon.svg`, favicon);
  await emit(`${web}/favicon.ico`, readFileSync(path.join(root, site, "favicon.ico")));
  await emit(`${web}/apple-icon.png`, readFileSync(path.join(root, site, "apple-touch-icon.png")));
  await emit(`${web}/opengraph-image.png`, homeCard);
  await emit(`${web}/opengraph-image.alt.txt`, "simhook: turn an Android phone into an SMS API\n");

  const drawable = "android/app/src/main/res/drawable";
  await emit(`${drawable}/ic_launcher_foreground.xml`, launcherForeground);
  await emit(`${drawable}/ic_notification.xml`, notificationIcon);

  if (check && drift.length) fail(`brand assets are stale, run pnpm brand:\n  ${drift.join("\n  ")}`);
  if (check) log("brand assets match the mark and the page titles");
}

main().catch((err) => fail(err?.stack ?? String(err)));
