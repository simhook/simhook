package dev.simhook.app.outbox

import android.content.Context
import androidx.room.ColumnInfo
import androidx.room.Dao
import androidx.room.Database
import androidx.room.Entity
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.PrimaryKey
import androidx.room.Query
import androidx.room.Room
import androidx.room.RoomDatabase
import androidx.room.migration.Migration
import androidx.sqlite.db.SupportSQLiteDatabase
import kotlinx.coroutines.flow.Flow

/**
 * A message the server asked this phone to send. Rows survive process death,
 * so a message fetched while the phone is busy is never lost.
 *
 * State machine: pending → sending → handed → sent | failed, with two side
 * paths. "sending" is the drainer's claim, taken in one conditional update
 * so no second loop can hand the same message to the radio. "handed" means
 * the row knows how many parts the radio was given and is waiting for the
 * sent broadcast of each. A radio that stays silent moves the row to
 * "awaiting", and only after a long horizon to "interrupted", which is
 * reported to the server as a failure the truth may still overturn: a late
 * broadcast still counts, on the phone and on the server. A condition the
 * radio can recover from (no service, the platform's rate limit) puts the
 * row back to pending with a cool-off in [notBefore] rather than calling
 * it failed. Every step is one conditional update, so two broadcasts
 * arriving together cannot count the same part twice or finish a message
 * twice.
 */
@Entity(tableName = "outbox")
data class OutboxMessage(
    @PrimaryKey val id: String,
    val batchId: String,
    val to: String,
    val body: String,
    val simSubscriptionId: Int?,
    val state: String = STATE_PENDING,
    val parts: Int = 1,
    val partsOk: Int = 0,
    val attempts: Int = 0,
    val createdAt: Long,
    val updatedAt: Long = createdAt,
    val lastError: String? = null,
    /** Not sent again before this time; the cool-off after a recoverable failure. */
    @ColumnInfo(defaultValue = "0") val notBefore: Long = 0L,
) {
    companion object {
        const val STATE_PENDING = "pending"
        const val STATE_SENDING = "sending"
        const val STATE_HANDED = "handed"
        const val STATE_AWAITING = "awaiting"
        const val STATE_SENT = "sent"
        const val STATE_FAILED = "failed"
        const val STATE_INTERRUPTED = "interrupted"
    }
}

@Dao
interface OutboxDao {
    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insert(message: OutboxMessage): Long

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insertAll(messages: List<OutboxMessage>): List<Long>

    /** The oldest message that may go now: pending and past its cool-off. */
    @Query("SELECT * FROM outbox WHERE state = 'pending' AND notBefore <= :now ORDER BY createdAt ASC LIMIT 1")
    suspend fun nextPending(now: Long): OutboxMessage?

    /** When the earliest cooling-off message may go, or null when none is cooling off. */
    @Query("SELECT MIN(notBefore) FROM outbox WHERE state = 'pending' AND notBefore > :now")
    suspend fun earliestNotBefore(now: Long): Long?

    @Query("SELECT * FROM outbox WHERE id = :id")
    suspend fun get(id: String): OutboxMessage?

    @Query("SELECT COUNT(*) FROM outbox WHERE state = 'pending'")
    suspend fun pendingCount(): Int

    @Query("SELECT COUNT(*) FROM outbox WHERE state IN ('pending', 'sending', 'handed', 'awaiting')")
    suspend fun inFlightCount(): Int

    @Query("SELECT COUNT(*) FROM outbox WHERE state IN ('pending', 'sending', 'handed', 'awaiting')")
    fun inFlightCountFlow(): Flow<Int>

    @Query("SELECT * FROM outbox ORDER BY createdAt DESC LIMIT 200")
    fun recent(): Flow<List<OutboxMessage>>

    /** Segments the radio was given since [since]; seeds the rate budget after a restart. */
    @Query("SELECT COALESCE(SUM(parts), 0) FROM outbox WHERE state IN ('handed', 'awaiting', 'sent', 'failed', 'interrupted') AND updatedAt > :since")
    suspend fun segmentsSince(since: Long): Int

    @Query("UPDATE outbox SET state = :state, updatedAt = :now, lastError = :error WHERE id = :id")
    suspend fun setState(id: String, state: String, now: Long, error: String?)

    /** Takes a pending message for sending. Exactly one caller sees 1. */
    @Query("UPDATE outbox SET state = 'sending', updatedAt = :now WHERE id = :id AND state = 'pending'")
    suspend fun claim(id: String, now: Long): Int

    /** A claim the process died on before the radio got anything goes back to the queue. */
    @Query("UPDATE outbox SET state = 'pending', updatedAt = :now WHERE state = 'sending' AND updatedAt < :before")
    suspend fun unclaim(before: Long, now: Long)

    /** The radio is about to get [parts] segments; from now on their broadcasts count. */
    @Query("UPDATE outbox SET state = 'handed', attempts = attempts + 1, parts = :parts, partsOk = 0, updatedAt = :now WHERE id = :id AND state = 'sending'")
    suspend fun markHanded(id: String, parts: Int, now: Long): Int

    /** The radio has had its time to answer; the row waits on, off the drainer's critical path. */
    @Query("UPDATE outbox SET state = 'awaiting', updatedAt = :now WHERE id = :id AND state = 'handed'")
    suspend fun await(id: String, now: Long): Int

    /** One segment went out. Counts while the radio's word is still worth having, however late. */
    @Query("UPDATE outbox SET partsOk = partsOk + 1, updatedAt = :now WHERE id = :id AND state IN ('handed', 'awaiting', 'interrupted')")
    suspend fun partOk(id: String, now: Long): Int

    /** Finishes the message once every segment is out. Exactly one caller sees 1. */
    @Query("UPDATE outbox SET state = 'sent', updatedAt = :now WHERE id = :id AND state IN ('handed', 'awaiting', 'interrupted') AND partsOk >= parts")
    suspend fun completeIfAllParts(id: String, now: Long): Int

    /** Ends a message that is still in flight. 0 when it already ended. */
    @Query("UPDATE outbox SET state = :state, updatedAt = :now, lastError = :error WHERE id = :id AND state IN ('pending', 'sending', 'handed', 'awaiting')")
    suspend fun finish(id: String, state: String, now: Long, error: String?): Int

    /**
     * Puts a message the radio could not take right now back in the queue,
     * to be tried again after [notBefore]. Only a message none of whose
     * parts went out; once a part is on the air a retry would repeat it.
     */
    @Query("UPDATE outbox SET state = 'pending', notBefore = :notBefore, updatedAt = :now, lastError = :error WHERE id = :id AND state IN ('sending', 'handed', 'awaiting') AND partsOk = 0")
    suspend fun retryLater(id: String, notBefore: Long, now: Long, error: String?): Int

    /** Rows the radio has had for longer than any answer takes. */
    @Query("SELECT * FROM outbox WHERE state IN ('handed', 'awaiting') AND updatedAt < :before")
    suspend fun interrupted(before: Long): List<OutboxMessage>

    @Query("DELETE FROM outbox WHERE state IN ('sent', 'failed', 'interrupted') AND updatedAt < :before")
    suspend fun prune(before: Long)

    @Query("DELETE FROM outbox")
    suspend fun clear()
}

/**
 * Something the phone owes the server: what became of a message it sent, or
 * a text it received. A row stays until the server has taken it, however
 * long the network is away, so a status is never dropped and a received
 * text, which exists nowhere else, is never lost.
 */
@Entity(tableName = "reports")
data class PendingReport(
    @PrimaryKey(autoGenerate = true) val id: Long = 0L,
    val kind: String,
    /** Status reports: the message and what became of it. */
    val messageId: String? = null,
    val status: String? = null,
    val errorCode: String? = null,
    val errorMessage: String? = null,
    /** Inbound reports: the text as received. */
    val sender: String? = null,
    val body: String? = null,
    val fingerprint: String? = null,
    val simSubscriptionId: Int? = null,
    /** When it happened, as the phone saw it. */
    val at: Long,
    val createdAt: Long,
    val attempts: Int = 0,
) {
    companion object {
        const val KIND_STATUS = "status"
        const val KIND_INBOUND = "inbound"
    }
}

@Dao
interface ReportDao {
    @Insert
    suspend fun insert(report: PendingReport): Long

    @Query("SELECT * FROM reports ORDER BY id ASC LIMIT :limit")
    suspend fun oldest(limit: Int): List<PendingReport>

    @Query("SELECT COUNT(*) FROM reports")
    suspend fun count(): Int

    @Query("UPDATE reports SET attempts = attempts + 1 WHERE id = :id")
    suspend fun bump(id: Long)

    @Query("DELETE FROM reports WHERE id = :id")
    suspend fun delete(id: Long)

    @Query("DELETE FROM reports")
    suspend fun clear()
}

@Database(entities = [OutboxMessage::class, PendingReport::class], version = 2, exportSchema = false)
abstract class AppDatabase : RoomDatabase() {
    abstract fun outbox(): OutboxDao
    abstract fun reports(): ReportDao

    companion object {
        @Volatile
        private var instance: AppDatabase? = null

        /** Version 2 added the cool-off column and the ledger of what the phone owes the server. */
        private val MIGRATION_1_2 = object : Migration(1, 2) {
            override fun migrate(db: SupportSQLiteDatabase) {
                db.execSQL("ALTER TABLE `outbox` ADD COLUMN `notBefore` INTEGER NOT NULL DEFAULT 0")
                db.execSQL(
                    "CREATE TABLE IF NOT EXISTS `reports` (" +
                        "`id` INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, `kind` TEXT NOT NULL, " +
                        "`messageId` TEXT, `status` TEXT, `errorCode` TEXT, `errorMessage` TEXT, " +
                        "`sender` TEXT, `body` TEXT, `fingerprint` TEXT, `simSubscriptionId` INTEGER, " +
                        "`at` INTEGER NOT NULL, `createdAt` INTEGER NOT NULL, `attempts` INTEGER NOT NULL)",
                )
            }
        }

        fun get(context: Context): AppDatabase = instance ?: synchronized(this) {
            instance ?: Room.databaseBuilder(context.applicationContext, AppDatabase::class.java, "simhook.db")
                .addMigrations(MIGRATION_1_2)
                .fallbackToDestructiveMigration(dropAllTables = true)
                .build()
                .also { instance = it }
        }
    }
}
