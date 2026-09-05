package dev.simhook.app.outbox

import android.content.Context
import androidx.room.Dao
import androidx.room.Database
import androidx.room.Entity
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.PrimaryKey
import androidx.room.Query
import androidx.room.Room
import androidx.room.RoomDatabase
import kotlinx.coroutines.flow.Flow

/**
 * A message the server asked this phone to send. Rows survive process death,
 * so a message fetched while the phone is busy is never lost.
 *
 * State machine: pending → handed → sent | failed. "handed" means the row
 * knows how many parts the radio was given and is waiting for the sent
 * broadcast of each; the receiver moves it on from there. Every step is
 * one conditional update, so two broadcasts arriving together cannot
 * count the same part twice or finish a message twice.
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
) {
    companion object {
        const val STATE_PENDING = "pending"
        const val STATE_SENDING = "sending"
        const val STATE_HANDED = "handed"
        const val STATE_SENT = "sent"
        const val STATE_FAILED = "failed"
    }
}

@Dao
interface OutboxDao {
    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insert(message: OutboxMessage): Long

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insertAll(messages: List<OutboxMessage>): List<Long>

    @Query("SELECT * FROM outbox WHERE state = 'pending' ORDER BY createdAt ASC LIMIT 1")
    suspend fun nextPending(): OutboxMessage?

    @Query("SELECT * FROM outbox WHERE id = :id")
    suspend fun get(id: String): OutboxMessage?

    @Query("SELECT COUNT(*) FROM outbox WHERE state = 'pending'")
    suspend fun pendingCount(): Int

    @Query("SELECT COUNT(*) FROM outbox WHERE state IN ('pending', 'sending', 'handed')")
    suspend fun inFlightCount(): Int

    @Query("SELECT COUNT(*) FROM outbox WHERE state IN ('pending', 'sending', 'handed')")
    fun inFlightCountFlow(): Flow<Int>

    @Query("SELECT * FROM outbox ORDER BY createdAt DESC LIMIT 200")
    fun recent(): Flow<List<OutboxMessage>>

    @Query("UPDATE outbox SET state = :state, updatedAt = :now, lastError = :error WHERE id = :id")
    suspend fun setState(id: String, state: String, now: Long, error: String?)

    /** The radio is about to get [parts] segments; from now on their broadcasts count. */
    @Query("UPDATE outbox SET state = 'handed', attempts = attempts + 1, parts = :parts, partsOk = 0, updatedAt = :now WHERE id = :id")
    suspend fun markHanded(id: String, parts: Int, now: Long)

    /** One segment went out. Counts only while the message is waiting on the radio. */
    @Query("UPDATE outbox SET partsOk = partsOk + 1, updatedAt = :now WHERE id = :id AND state = 'handed'")
    suspend fun partOk(id: String, now: Long): Int

    /** Finishes the message once every segment is out. Exactly one caller sees 1. */
    @Query("UPDATE outbox SET state = 'sent', updatedAt = :now WHERE id = :id AND state = 'handed' AND partsOk >= parts")
    suspend fun completeIfAllParts(id: String, now: Long): Int

    /** Ends a message that is still in flight. 0 when it already ended. */
    @Query("UPDATE outbox SET state = :state, updatedAt = :now, lastError = :error WHERE id = :id AND state IN ('pending', 'sending', 'handed')")
    suspend fun finish(id: String, state: String, now: Long, error: String?): Int

    /** Rows stuck mid-send from a previous process life. */
    @Query("SELECT * FROM outbox WHERE state IN ('sending', 'handed') AND updatedAt < :before")
    suspend fun interrupted(before: Long): List<OutboxMessage>

    @Query("DELETE FROM outbox WHERE state IN ('sent', 'failed') AND updatedAt < :before")
    suspend fun prune(before: Long)

    @Query("DELETE FROM outbox")
    suspend fun clear()
}

@Database(entities = [OutboxMessage::class], version = 1, exportSchema = false)
abstract class AppDatabase : RoomDatabase() {
    abstract fun outbox(): OutboxDao

    companion object {
        @Volatile
        private var instance: AppDatabase? = null

        fun get(context: Context): AppDatabase = instance ?: synchronized(this) {
            instance ?: Room.databaseBuilder(context.applicationContext, AppDatabase::class.java, "simhook.db")
                .fallbackToDestructiveMigration(dropAllTables = true)
                .build()
                .also { instance = it }
        }
    }
}
