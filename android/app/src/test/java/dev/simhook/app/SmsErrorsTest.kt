package dev.simhook.app

import android.app.Activity
import android.telephony.SmsManager
import dev.simhook.app.sms.SmsErrors
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class SmsErrorsTest {
    @Test
    fun sentResultsHaveStableCodes() {
        assertEquals("no_service", SmsErrors.forSentResult(SmsManager.RESULT_ERROR_NO_SERVICE, null).code)
        assertEquals("radio_off", SmsErrors.forSentResult(SmsManager.RESULT_ERROR_RADIO_OFF, null).code)
        assertEquals("rate_limited", SmsErrors.forSentResult(SmsManager.RESULT_ERROR_LIMIT_EXCEEDED, null).code)
        assertEquals("cancelled", SmsErrors.forSentResult(Activity.RESULT_CANCELED, null).code)
        assertEquals("sms_error_999", SmsErrors.forSentResult(999, null).code)
    }

    @Test
    fun radioCodeIsAppended() {
        val f = SmsErrors.forSentResult(SmsManager.RESULT_ERROR_GENERIC_FAILURE, 42)
        assertEquals("generic_failure", f.code)
        assertTrue(f.message.endsWith("(radio code 42)"))
    }

    @Test
    fun deliveryFollowsTheStatusReport() {
        assertEquals(SmsErrors.DeliveryOutcome.Delivered, SmsErrors.classifyDelivery(Activity.RESULT_OK, 0x00))
        assertEquals(SmsErrors.DeliveryOutcome.Delivered, SmsErrors.classifyDelivery(Activity.RESULT_CANCELED, 0x02))
        assertEquals(SmsErrors.DeliveryOutcome.Pending, SmsErrors.classifyDelivery(Activity.RESULT_OK, 0x20))
        assertEquals(SmsErrors.DeliveryOutcome.Pending, SmsErrors.classifyDelivery(Activity.RESULT_OK, 0x3F))
        assertEquals(SmsErrors.DeliveryOutcome.Failed, SmsErrors.classifyDelivery(Activity.RESULT_OK, 0x40))
        assertEquals(SmsErrors.DeliveryOutcome.Failed, SmsErrors.classifyDelivery(Activity.RESULT_OK, 0x41))
    }

    @Test
    fun deliveryWithoutAReportFallsBackToTheResultCode() {
        assertEquals(SmsErrors.DeliveryOutcome.Delivered, SmsErrors.classifyDelivery(Activity.RESULT_OK, null))
        assertEquals(SmsErrors.DeliveryOutcome.Pending, SmsErrors.classifyDelivery(Activity.RESULT_CANCELED, null))
        assertEquals(SmsErrors.DeliveryOutcome.Failed, SmsErrors.classifyDelivery(7, null))
        assertEquals("delivery_failed", SmsErrors.deliveryFailure(7, null).code)
        assertTrue(SmsErrors.deliveryFailure(Activity.RESULT_OK, 0x41).message.contains("0x41"))
    }
}
