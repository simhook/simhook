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
 * so a push that arrives while the phone is busy is never lost.
 *
 * State machine: pending → sending → handed → sent | failed.
 * "handed" means the radio accepted it and we are waiting for the sent
 * broadcast; the receiver moves it on from there.
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

    @Query("SELECT * FROM outbox WHERE state = 'pending' ORDER BY createdAt ASC LIMIT 1")
    suspend fun nextPending(): OutboxMessage?

    @Query("SELECT * FROM outbox WHERE id = :id")
    suspend fun get(id: String): OutboxMessage?

    @Query("SELECT COUNT(*) FROM outbox WHERE state IN ('pending', 'sending', 'handed')")
    suspend fun inFlightCount(): Int

    @Query("SELECT COUNT(*) FROM outbox WHERE state IN ('pending', 'sending', 'handed')")
    fun inFlightCountFlow(): Flow<Int>

    @Query("SELECT * FROM outbox ORDER BY createdAt DESC LIMIT 200")
    fun recent(): Flow<List<OutboxMessage>>

    @Query("UPDATE outbox SET state = :state, updatedAt = :now, lastError = :error WHERE id = :id")
    suspend fun setState(id: String, state: String, now: Long, error: String?)

    @Query("UPDATE outbox SET state = 'sending', attempts = attempts + 1, parts = :parts, partsOk = 0, updatedAt = :now WHERE id = :id")
    suspend fun markSending(id: String, parts: Int, now: Long)

    @Query("UPDATE outbox SET partsOk = partsOk + 1, updatedAt = :now WHERE id = :id")
    suspend fun partOk(id: String, now: Long)

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
