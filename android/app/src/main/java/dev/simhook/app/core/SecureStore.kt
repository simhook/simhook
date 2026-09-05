package dev.simhook.app.core

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.io.IOException
import java.security.KeyStore
import javax.crypto.AEADBadTagException
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * The secure store could not answer right now: the keystore was busy, locked,
 * or briefly unavailable. The secret is still there; ask again later.
 */
class TokenUnavailableException(cause: Throwable) : IOException("The secure store is not available right now", cause)

/**
 * Small secrets (the device token) encrypted with an AES-256-GCM key that
 * lives in the Android Keystore and never leaves the secure hardware.
 * Ciphertexts are stored in ordinary SharedPreferences; without the key
 * they are useless.
 *
 * Only two things mean a secret is gone for good: the key no longer exists
 * (a wiped keystore after a factory reset or key invalidation), or the
 * ciphertext no longer matches it. Anything else is a passing failure and
 * is reported as such, so a busy keystore never unpairs the phone.
 */
class SecureStore(context: Context) {
    private val prefs = context.getSharedPreferences("secure", Context.MODE_PRIVATE)

    fun put(name: String, value: String) {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, keyForWriting())
        val iv = cipher.iv
        val ct = cipher.doFinal(value.toByteArray(Charsets.UTF_8))
        val blob = ByteArray(1 + iv.size + ct.size)
        blob[0] = iv.size.toByte()
        iv.copyInto(blob, 1)
        ct.copyInto(blob, 1 + iv.size)
        prefs.edit().putString(name, Base64.encodeToString(blob, Base64.NO_WRAP)).apply()
    }

    /** The secret, null when none is stored or it can never be read again, or [TokenUnavailableException] when the store is busy. */
    @Throws(TokenUnavailableException::class)
    fun get(name: String): String? {
        val encoded = prefs.getString(name, null) ?: return null
        val key = try {
            existingKey()
        } catch (e: Exception) {
            throw TokenUnavailableException(e)
        }
        if (key == null) {
            // The key is gone, so the blob can never be read: forget it.
            prefs.edit().remove(name).apply()
            return null
        }
        return try {
            val blob = Base64.decode(encoded, Base64.NO_WRAP)
            val ivLen = blob[0].toInt()
            val iv = blob.copyOfRange(1, 1 + ivLen)
            val ct = blob.copyOfRange(1 + ivLen, blob.size)
            val cipher = Cipher.getInstance(TRANSFORMATION)
            cipher.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(128, iv))
            String(cipher.doFinal(ct), Charsets.UTF_8)
        } catch (e: AEADBadTagException) {
            prefs.edit().remove(name).apply()
            null
        } catch (e: IllegalArgumentException) {
            // Not something we wrote.
            prefs.edit().remove(name).apply()
            null
        } catch (e: Exception) {
            throw TokenUnavailableException(e)
        }
    }

    fun remove(name: String) {
        prefs.edit().remove(name).apply()
    }

    private fun existingKey(): SecretKey? {
        val ks = KeyStore.getInstance(KEYSTORE).apply { load(null) }
        return ks.getKey(KEY_ALIAS, null) as? SecretKey
    }

    private fun keyForWriting(): SecretKey {
        existingKey()?.let { return it }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, KEYSTORE)
        generator.init(
            KeyGenParameterSpec.Builder(KEY_ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .build(),
        )
        return generator.generateKey()
    }

    companion object {
        private const val KEYSTORE = "AndroidKeyStore"
        private const val KEY_ALIAS = "simhook.secure.v1"
        private const val TRANSFORMATION = "AES/GCM/NoPadding"

        const val DEVICE_TOKEN = "device_token"
    }
}
