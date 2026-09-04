package dev.simhook.app.sms

import android.app.Activity
import android.telephony.SmsManager

/** A send failure the way the API and a human want to see it. */
data class SmsFailure(val code: String, val message: String)

/** Translates platform result codes into stable codes and plain-language advice. */
object SmsErrors {
    fun forSentResult(resultCode: Int, radioError: Int?): SmsFailure {
        val base = when (resultCode) {
            SmsManager.RESULT_ERROR_GENERIC_FAILURE -> SmsFailure(
                "generic_failure",
                "The phone could not send the message. Usually the SIM has no credit, is out of coverage, or the carrier refused it.",
            )
            SmsManager.RESULT_ERROR_RADIO_OFF -> SmsFailure("radio_off", "The mobile radio is off. Turn off airplane mode and make sure mobile service is on.")
            SmsManager.RESULT_ERROR_NULL_PDU -> SmsFailure("invalid_message", "The message could not be encoded for this network.")
            SmsManager.RESULT_ERROR_NO_SERVICE -> SmsFailure("no_service", "No mobile service. The phone has no signal right now.")
            SmsManager.RESULT_ERROR_LIMIT_EXCEEDED -> SmsFailure(
                "rate_limited",
                "Android's outgoing SMS limit for apps was reached. Increase the send delay in settings, or raise the limit on this phone.",
            )
            SmsManager.RESULT_ERROR_FDN_CHECK_FAILURE -> SmsFailure("fdn_blocked", "The SIM's fixed dialling list blocks this number.")
            SmsManager.RESULT_ERROR_SHORT_CODE_NOT_ALLOWED,
            SmsManager.RESULT_ERROR_SHORT_CODE_NEVER_ALLOWED,
            -> SmsFailure("short_code_blocked", "Sending to short codes is blocked on this phone. Use a full phone number.")
            SmsManager.RESULT_RADIO_NOT_AVAILABLE -> SmsFailure("radio_unavailable", "The mobile radio is not available.")
            SmsManager.RESULT_NETWORK_REJECT -> SmsFailure("network_rejected", "The carrier network rejected the message.")
            SmsManager.RESULT_INVALID_ARGUMENTS, SmsManager.RESULT_INVALID_SMS_FORMAT, SmsManager.RESULT_ENCODING_ERROR ->
                SmsFailure("invalid_message", "The message or number is not valid for this network.")
            SmsManager.RESULT_INVALID_STATE, SmsManager.RESULT_MODEM_ERROR, SmsManager.RESULT_SYSTEM_ERROR, SmsManager.RESULT_INTERNAL_ERROR ->
                SmsFailure("phone_error", "The phone's messaging stack reported an internal error. Try again.")
            SmsManager.RESULT_NETWORK_ERROR -> SmsFailure("network_error", "A network error interrupted the send. Check signal and try again.")
            SmsManager.RESULT_INVALID_SMSC_ADDRESS -> SmsFailure("invalid_smsc", "The SIM has no valid message centre number configured.")
            SmsManager.RESULT_OPERATION_NOT_ALLOWED, SmsManager.RESULT_REQUEST_NOT_SUPPORTED ->
                SmsFailure("not_allowed", "This phone or SIM does not allow sending SMS this way.")
            SmsManager.RESULT_NO_MEMORY, SmsManager.RESULT_NO_RESOURCES -> SmsFailure("phone_busy", "The phone ran out of resources while sending. Try again.")
            SmsManager.RESULT_CANCELLED -> SmsFailure("cancelled", "The send was cancelled by the phone.")
            Activity.RESULT_CANCELED -> SmsFailure("cancelled", "The send was cancelled by the phone.")
            else -> SmsFailure("sms_error_$resultCode", "The phone reported error $resultCode while sending.")
        }
        return if (radioError != null && radioError > 0) base.copy(message = "${base.message} (radio code $radioError)") else base
    }

    fun forDeliveryResult(resultCode: Int): SmsFailure? = when (resultCode) {
        Activity.RESULT_OK -> null
        Activity.RESULT_CANCELED -> null // no delivery report from this carrier; not a failure
        else -> SmsFailure("delivery_failed", "The carrier reported the message was not delivered (code $resultCode).")
    }
}
