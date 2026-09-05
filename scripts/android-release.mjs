#!/usr/bin/env node
// Builds, verifies, and publishes a signed release of the Android app.
//
//   node scripts/android-release.mjs                 build, verify, write the manifest (nothing leaves this machine)
//   node scripts/android-release.mjs --publish       ...and create the GitHub release with the APK and manifest
//
// Options
//   --repo owner/name        releases repository (default: $SIMHOOK_RELEASES_REPO or simhook/releases)
//   --notes-file path        release notes in Markdown; the first paragraph also goes into the manifest
//   --min-code N             oldest version code the server still supports (default 1)
//   --apk-base-url url       where the APK will be downloadable from (default: the GitHub release)
//   --cert-sha256 hex        expected signing certificate fingerprint (default: the simhook release key)
//   --skip-build             reuse app-release.apk from the previous build
//   --force                  replace an existing release for this version
//
// The version comes from android/app/build.gradle.kts. Bump it there, commit, then run this.
// See android/RELEASING.md.
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { copyFileSync, existsSync, mkdirSync, readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const SIMHOOK_RELEASE_CERT_SHA256 = "275f52c9a68f1dfc2c1e679e82cbcc860213ac35b9a9f1ff180ecd16e37eaaf6";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const android = path.join(root, "android");
const win = process.platform === "win32";
const args = parseArgs(process.argv.slice(2));
const repo = args.repo ?? process.env.SIMHOOK_RELEASES_REPO ?? "simhook/releases";
const minCode = Number(args["min-code"] ?? 1);
const expectedCert = (args["cert-sha256"] ?? SIMHOOK_RELEASE_CERT_SHA256).toLowerCase();

// 1. Version, from the Gradle file so the APK and the manifest cannot disagree.
const gradle = readFileSync(path.join(android, "app", "build.gradle.kts"), "utf8");
const versionCode = Number(gradle.match(/versionCode = prop\("versionCode"\)\?\.toInt\(\) \?: (\d+)/)?.[1]);
const versionName = gradle.match(/versionName = prop\("versionName"\) \?: "([^"]+)"/)?.[1];
if (!versionCode || !versionName) fail("could not read versionCode/versionName from android/app/build.gradle.kts");
const tag = `android-v${versionName}`;
log(`version ${versionName} (code ${versionCode}), tag ${tag}, repo ${repo}`);

// 2. Tools.
const sdk = sdkDir();
const buildTools = latestBuildTools(sdk);
const apksigner = path.join(buildTools, win ? "apksigner.bat" : "apksigner");
const aapt2 = path.join(buildTools, win ? "aapt2.exe" : "aapt2");
if (!existsSync(apksigner) || !existsSync(aapt2)) fail(`apksigner or aapt2 missing in ${buildTools}`);
if (!existsSync(path.join(android, "keystore.properties")) && !process.env.SIMHOOK_KEYSTORE_FILE) {
  fail("no android/keystore.properties and no SIMHOOK_KEYSTORE_FILE: release builds would be unsigned");
}

// 3. Build.
const apkDir = path.join(android, "app", "build", "outputs", "apk", "release");
const builtApk = path.join(apkDir, "app-release.apk");
if (!args["skip-build"]) {
  log("building the release APK");
  run(path.join(android, win ? "gradlew.bat" : "gradlew"), [":app:assembleRelease", "--console=plain"], { cwd: android, shell: win });
}
if (!existsSync(builtApk)) {
  if (existsSync(path.join(apkDir, "app-release-unsigned.apk"))) fail("the build produced an unsigned APK; check keystore.properties");
  fail(`no APK at ${builtApk}`);
}

// 4. Verify: signature, signer, package, version.
const verify = capture(apksigner, ["verify", "--print-certs", "-v", builtApk], { shell: win });
if (!/Verified using v2 scheme \(APK Signature Scheme v2\): true/.test(verify)) fail(`APK is not v2-signed:\n${verify}`);
const cert = verify.match(/Signer #1 certificate SHA-256 digest: ([0-9a-f]{64})/)?.[1];
if (cert !== expectedCert) fail(`APK is signed with ${cert}, expected ${expectedCert}`);
const badging = capture(aapt2, ["dump", "badging", builtApk]);
const pkg = badging.match(/package: name='([^']+)' versionCode='(\d+)' versionName='([^']+)'/);
if (!pkg) fail("could not read package info from the APK");
if (pkg[1] !== "dev.simhook.app") fail(`unexpected package ${pkg[1]}`);
if (Number(pkg[2]) !== versionCode || pkg[3] !== versionName) {
  fail(`APK is ${pkg[3]} (code ${pkg[2]}) but build.gradle.kts says ${versionName} (code ${versionCode}); rebuild without --skip-build`);
}
const targetSdk = badging.match(/targetSdkVersion:'(\d+)'/)?.[1];
if (targetSdk !== "36") fail(`targetSdk is ${targetSdk}; it must stay 36 (docs/decisions.md 009)`);
log(`signature ok, signer ${cert.slice(0, 16)}..., ${pkg[1]} ${pkg[3]} (${pkg[2]}), targetSdk ${targetSdk}`);

// 5. Release files.
const outDir = path.join(android, "app", "build", "outputs", "release", versionName);
mkdirSync(outDir, { recursive: true });
const apkName = `simhook-${versionName}.apk`;
const apkPath = path.join(outDir, apkName);
copyFileSync(builtApk, apkPath);
copyFileSync(builtApk, path.join(outDir, "simhook.apk"));
const bytes = readFileSync(apkPath);
const sha256 = createHash("sha256").update(bytes).digest("hex");
const notesText = args["notes-file"] ? readFileSync(path.resolve(args["notes-file"]), "utf8").trim() : "";
const apkBase = (args["apk-base-url"] ?? `https://github.com/${repo}/releases/download/${tag}`).replace(/\/$/, "");
const manifest = {
  version_code: versionCode,
  version_name: versionName,
  min_supported_version_code: minCode,
  apk_url: `${apkBase}/${apkName}`,
  sha256,
  size_bytes: bytes.length,
  released_at: new Date().toISOString(),
  notes: firstParagraph(notesText),
};
writeFileSync(path.join(outDir, "android.json"), JSON.stringify(manifest, null, 2) + "\n");
writeFileSync(path.join(outDir, "SHA256SUMS"), `${sha256}  ${apkName}\n${sha256}  simhook.apk\n`);
log(`wrote ${path.relative(root, outDir)}: ${apkName} (${(bytes.length / 1048576).toFixed(1)} MB), simhook.apk, android.json, SHA256SUMS`);
log(`sha256 ${sha256}`);

// 6. Publish.
if (!args.publish) {
  log("dry run: pass --publish to create the GitHub release");
  process.exit(0);
}
const exists = spawnSync("gh", ["release", "view", tag, "-R", repo], { stdio: "ignore", shell: win }).status === 0;
if (exists && !args.force) fail(`release ${tag} already exists in ${repo}; pass --force to replace it`);
if (exists) {
  log(`deleting the existing ${tag} release`);
  run("gh", ["release", "delete", tag, "-R", repo, "--yes", "--cleanup-tag"], { shell: win });
}
const notesFile = path.join(outDir, "notes.md");
writeFileSync(
  notesFile,
  (notesText || `simhook for Android ${versionName}.`) +
    `\n\nInstall \`${apkName}\` on the phone, or let an installed app update itself.\n\nSHA-256: \`${sha256}\`\n`,
);
run(
  "gh",
  [
    "release", "create", tag, "-R", repo,
    "--title", `simhook for Android ${versionName}`,
    "--notes-file", notesFile,
    "--latest",
    apkPath, path.join(outDir, "simhook.apk"), path.join(outDir, "android.json"), path.join(outDir, "SHA256SUMS"),
  ],
  { shell: win },
);
log(`published https://github.com/${repo}/releases/tag/${tag}`);
log(`latest manifest: https://github.com/${repo}/releases/latest/download/android.json`);
log(`latest APK:      https://github.com/${repo}/releases/latest/download/simhook.apk`);

// ---------------------------------------------------------------------------

function parseArgs(argv) {
  const out = {};
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (!a.startsWith("--")) fail(`unexpected argument ${a}`);
    const key = a.slice(2);
    const flag = ["publish", "skip-build", "force"].includes(key);
    out[key] = flag ? true : argv[++i];
    if (!flag && out[key] === undefined) fail(`--${key} needs a value`);
  }
  return out;
}

function sdkDir() {
  const local = path.join(android, "local.properties");
  if (existsSync(local)) {
    const m = readFileSync(local, "utf8").match(/^sdk\.dir=(.+)$/m);
    if (m) return m[1].trim().replace(/\\:/g, ":").replace(/\\\\/g, "\\");
  }
  const env = process.env.ANDROID_HOME ?? process.env.ANDROID_SDK_ROOT;
  if (!env) fail("Android SDK not found: set sdk.dir in android/local.properties or ANDROID_HOME");
  return env;
}

function latestBuildTools(sdk) {
  const dir = path.join(sdk, "build-tools");
  const versions = readdirSync(dir)
    .filter((v) => /^\d+\.\d+\.\d+$/.test(v) && statSync(path.join(dir, v)).isDirectory())
    .sort((a, b) => {
      const [a1, a2, a3] = a.split(".").map(Number);
      const [b1, b2, b3] = b.split(".").map(Number);
      return b1 - a1 || b2 - a2 || b3 - a3;
    });
  if (versions.length === 0) fail(`no build-tools in ${dir}`);
  return path.join(dir, versions[0]);
}

function quoteIfShell(cmdArgs, opts) {
  return opts.shell ? cmdArgs.map((a) => (/[\s"]/.test(a) ? `"${a.replace(/"/g, '\\"')}"` : a)) : cmdArgs;
}

function run(cmd, cmdArgs, opts = {}) {
  const r = spawnSync(cmd, quoteIfShell(cmdArgs, opts), { stdio: "inherit", ...opts });
  if (r.error) fail(`${cmd}: ${r.error.message}`);
  if (r.status !== 0) fail(`${cmd} exited with ${r.status}`);
}

function capture(cmd, cmdArgs, opts = {}) {
  const r = spawnSync(cmd, quoteIfShell(cmdArgs, opts), { encoding: "utf8", ...opts });
  if (r.error) fail(`${cmd}: ${r.error.message}`);
  if (r.status !== 0) fail(`${cmd} exited with ${r.status}\n${r.stdout}\n${r.stderr}`);
  return r.stdout + r.stderr;
}

function firstParagraph(text) {
  const p = text.split(/\n\s*\n/)[0]?.replace(/^#+\s*/gm, "").trim();
  return p ? p.slice(0, 500) : null;
}

function log(msg) {
  console.log(`[release] ${msg}`);
}

function fail(msg) {
  console.error(`[release] ${msg}`);
  process.exit(1);
}
