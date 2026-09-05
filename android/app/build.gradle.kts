import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.ksp)
}

// Push needs a Firebase config file that lives outside version control:
// src/debug/google-services.json for the development project and
// src/release/google-services.json for the production one, so a debug build
// can never reach production phones. The app builds and runs without them;
// only pushes are unavailable until they exist.
val googleServicesFiles = listOf("google-services.json", "src/debug/google-services.json", "src/release/google-services.json").map(::file)
if (googleServicesFiles.any { it.exists() }) {
    apply(plugin = libs.plugins.google.services.get().pluginId)
} else {
    logger.warn("no google-services.json found: building without push support")
}

// Release signing. The keystore and its passwords never enter the repository:
// android/keystore.properties (gitignored) points at them, or the four
// SIMHOOK_KEYSTORE_* variables do on a build machine. Without either, release
// builds come out unsigned, which is fine for CI compile checks.
val keystoreProps = Properties().apply {
    val f = rootProject.file("keystore.properties")
    if (f.exists()) f.inputStream().use { load(it) }
}
fun signing(key: String, env: String): String? = keystoreProps.getProperty(key) ?: System.getenv(env)
val releaseStoreFile = signing("storeFile", "SIMHOOK_KEYSTORE_FILE")

// The release script and CI can pin the version without editing this file:
//   ./gradlew assembleRelease -PversionCode=3 -PversionName=0.2.0
fun prop(name: String): String? = (findProperty(name) as String?)?.takeIf { it.isNotBlank() }

android {
    namespace = "dev.simhook.app"
    compileSdk = 37

    defaultConfig {
        applicationId = "dev.simhook.app"
        minSdk = 26
        // Stays at 36 on purpose: Android 17 withholds one-time-code texts from apps
        // targeting 37 for three hours unless they are the default SMS app. See docs/decisions.md 009.
        targetSdk = 36
        versionCode = prop("versionCode")?.toInt() ?: 3
        versionName = prop("versionName") ?: "0.1.1"
        vectorDrawables.useSupportLibrary = true

        // Where the app looks for newer builds. See docs/decisions.md 012.
        buildConfigField(
            "String",
            "UPDATE_MANIFEST_URL",
            "\"${prop("updateManifestUrl") ?: "https://simhook.dev/download/android.json"}\"",
        )
    }

    signingConfigs {
        if (releaseStoreFile != null) {
            create("release") {
                storeFile = file(releaseStoreFile)
                storePassword = signing("storePassword", "SIMHOOK_KEYSTORE_PASSWORD")
                keyAlias = signing("keyAlias", "SIMHOOK_KEY_ALIAS")
                keyPassword = signing("keyPassword", "SIMHOOK_KEY_PASSWORD")
                enableV1Signing = false
                enableV2Signing = true
                enableV3Signing = true
                enableV4Signing = false
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            if (releaseStoreFile != null) {
                signingConfig = signingConfigs.getByName("release")
            } else {
                logger.warn("no release keystore configured: release builds will be unsigned")
            }
        }
        debug {
            versionNameSuffix = "-debug"
        }
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    packaging {
        resources.excludes += setOf("/META-INF/{AL2.0,LGPL2.1}", "META-INF/versions/9/OSGI-INF/MANIFEST.MF")
    }

    // No opaque dependency metadata block in the APK; the build is reproducible from source.
    dependenciesInfo {
        includeInApk = false
        includeInBundle = false
    }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)

    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.material3)
    implementation(libs.compose.material.icons.core)
    debugImplementation(libs.compose.ui.tooling)

    implementation(libs.androidx.datastore.preferences)
    implementation(libs.androidx.work.runtime.ktx)
    implementation(libs.androidx.room.runtime)
    implementation(libs.androidx.room.ktx)
    ksp(libs.androidx.room.compiler)

    implementation(libs.okhttp)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.kotlinx.coroutines.play.services)

    implementation(platform(libs.firebase.bom))
    implementation(libs.firebase.messaging)
    implementation(libs.play.services.code.scanner)
}
