package dev.simhook.app

import androidx.work.ExistingWorkPolicy
import androidx.work.WorkInfo
import dev.simhook.app.work.ReportUploadWorker
import org.junit.Assert.assertEquals
import org.junit.Test

class UploadPolicyTest {
    @Test
    fun aRunInProgressIsKept() {
        assertEquals(ExistingWorkPolicy.KEEP, ReportUploadWorker.policyFor(listOf(WorkInfo.State.RUNNING)))
    }

    @Test
    fun aParkedRetryIsReplaced() {
        assertEquals(ExistingWorkPolicy.REPLACE, ReportUploadWorker.policyFor(listOf(WorkInfo.State.ENQUEUED)))
        assertEquals(ExistingWorkPolicy.REPLACE, ReportUploadWorker.policyFor(listOf(WorkInfo.State.BLOCKED)))
    }

    @Test
    fun nothingScheduledStartsFresh() {
        assertEquals(ExistingWorkPolicy.REPLACE, ReportUploadWorker.policyFor(emptyList()))
        assertEquals(ExistingWorkPolicy.REPLACE, ReportUploadWorker.policyFor(listOf(WorkInfo.State.SUCCEEDED)))
    }
}
