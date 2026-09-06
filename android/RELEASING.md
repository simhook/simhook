# Releasing the Android app

The app ships as a signed APK on the releases page, next to a small manifest that installed apps poll. No store is involved. This file covers the key, the version numbers, the script, and what the app does with the manifest.

## The signing key

Every release is signed with one key. Android only installs an update over an existing app when both carry the same signature, so this key is the thing to protect: lose it and every user has to uninstall and pair again; leak it and someone else can ship a build that installs over ours.

The key lives outside the repository. Gradle finds it through `android/keystore.properties` (gitignored):

```
storeFile=/absolute/path/to/simhook-release.jks
storePassword=...
keyAlias=simhook
keyPassword=...
```

On a build machine without that file, the same four values can come from `SIMHOOK_KEYSTORE_FILE`, `SIMHOOK_KEYSTORE_PASSWORD`, `SIMHOOK_KEY_ALIAS`, and `SIMHOOK_KEY_PASSWORD`. Without either, `assembleRelease` produces an unsigned APK, which is what CI builds to catch shrinker problems.

Keep the keystore and its passwords backed up somewhere that is not the development machine. The release script refuses to publish an APK signed by any other key; its fingerprint is pinned at the top of `scripts/android-release.mjs`.

## Push configuration

Firebase has two projects: `simhook-dev` for development and `simhook-prod` for production. Their config files live outside the repository at `android/app/src/debug/google-services.json` and `android/app/src/release/google-services.json`, so a debug build can only ever reach the development server's phones and a release build only production's. A machine without either file still builds; its APKs just cannot be woken by push. The server side of each project is its service account: `api/firebase-service-account.json` locally, `deploy/secrets/fcm.json` on the production host.

## Version numbers

Both live in `android/app/build.gradle.kts`:

- `versionCode` is an integer that goes up by one on every published build. Android and the update check compare only this.
- `versionName` is what people see: `MAJOR.MINOR.PATCH`.

The git tag is `android-v<versionName>`. Test builds can override both without editing the file:

```
./gradlew assembleRelease -PversionCode=7 -PversionName=0.3.0-rc1
```

## The icon

The launcher foreground and the notification icon under `app/src/main/res/drawable/` are written by `pnpm brand` from the mark, together with every icon on the site. Change the mark there, not in the XML; a build with stale drawables fails CI.

## Cutting a release

1. Bump `versionCode` and `versionName`, commit.
2. Build and verify without publishing:

   ```
   node scripts/android-release.mjs
   ```

   This runs `assembleRelease`, checks the signature, signer, package name, version, and target SDK, then writes `android/app/build/outputs/release/<version>/` with `simhook-<version>.apk`, `simhook.apk`, `android.json`, and `SHA256SUMS`.

3. Install that APK on a phone and run through pairing, a send, and an incoming message.
4. Publish:

   ```
   node scripts/android-release.mjs --skip-build --publish --notes-file notes.md
   ```

   The script creates the GitHub release `android-v<version>` in the releases repository (`--repo`, default `simhook/simhook`) with the four files and marks it latest. Nothing else changes: `https://simhook.dev/download/android.json` and `/download/simhook.apk` redirect to the latest release, so installed apps pick the new build up within twelve hours, or the next time someone opens the app.

5. Rebuild the site. Its download page and changelog read the release when the site is built, and Docker reuses the previous build when no site file changed, so on the host run:

   ```
   cd /opt/simhook/deploy
   docker compose -f docker-compose.prod.yaml build --no-cache site
   docker compose -f docker-compose.prod.yaml up -d site
   ```

If a build must not be offered to older apps, pass `--min-code <versionCode>`: apps below it show the update as required.

## The manifest

`android.json`, published with every release:

```json
{
  "version_code": 2,
  "version_name": "0.1.0",
  "min_supported_version_code": 1,
  "apk_url": "https://github.com/simhook/simhook/releases/download/android-v0.1.0/simhook-0.1.0.apk",
  "sha256": "…",
  "size_bytes": 8412345,
  "released_at": "2026-09-05T14:00:00.000Z",
  "notes": "First public build."
}
```

The app reads it from `BuildConfig.UPDATE_MANIFEST_URL` (`https://simhook.dev/download/android.json`; self-hosters rebuild with `-PupdateManifestUrl=...`). `apk_url` must be https and `sha256` must match the file byte for byte.

## What the app does

- Checks the manifest twice a day in the background and whenever the home screen opens, at most once every six hours.
- When `version_code` is higher than its own, shows an update card on the home screen and posts one quiet notification per version. If `min_supported_version_code` is higher too, the card says the update is required.
- "Download and install" fetches the APK through the system download manager into the app's own storage, hashes it, and rejects anything that does not match `sha256`. Then it opens the system installer, or leaves a notification that does when the app is in the background.
- A verified download stays on offer as "Install" until it is installed, so the one-time permission prompt or a dismissed installer does not cost another download. A checksum mismatch discards the file and re-reads the manifest.
- The first time, Android asks the user to allow installs from simhook. The installer refuses any APK whose signature differs from the installed app's.
- After the install, the app restarts its check-in and clears the offer.
