package dev.simhook.app

import dev.simhook.app.api.ApiException
import dev.simhook.app.work.ReportOutcome
import dev.simhook.app.work.ReportUploader
import org.junit.Assert.assertEquals
import org.junit.Test

class ReportOutcomeTest {
    private fun server(status: Int) = ApiException(status, "code", "message")

    @Test
    fun aServerFailureLeavesTheReportAndMovesOn() {
        assertEquals(ReportOutcome.SKIP, ReportUploader.outcome(server(500), attempts = 0))
        assertEquals(ReportOutcome.SKIP, ReportUploader.outcome(server(503), attempts = 50))
    }

    @Test
    fun aRefusalDropsTheReport() {
        assertEquals(ReportOutcome.DROP, ReportUploader.outcome(server(400), attempts = 0))
        assertEquals(ReportOutcome.DROP, ReportUploader.outcome(server(422), attempts = 0))
        assertEquals(ReportOutcome.DROP, ReportUploader.outcome(server(404), attempts = 0))
    }

    @Test
    fun backPressureEndsTheRun() {
        assertEquals(ReportOutcome.STOP, ReportUploader.outcome(server(429), attempts = 0))
        assertEquals(ReportOutcome.STOP, ReportUploader.outcome(server(408), attempts = 0))
    }

    @Test
    fun aReportThatNeverGetsThroughIsLetGoEventually() {
        assertEquals(ReportOutcome.SKIP, ReportUploader.outcome(server(500), attempts = ReportUploader.MAX_ATTEMPTS - 2))
        assertEquals(ReportOutcome.DROP, ReportUploader.outcome(server(500), attempts = ReportUploader.MAX_ATTEMPTS - 1))
    }

    @Test
    fun aDeadTokenEndsEverything() {
        assertEquals(ReportOutcome.LOST_PAIRING, ReportUploader.outcome(server(401), attempts = 0))
        assertEquals(ReportOutcome.DROP, ReportUploader.outcome(ApiException(401, "code", "message", fromServer = false), attempts = 0))
    }
}
