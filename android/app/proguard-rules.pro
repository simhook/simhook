# kotlinx.serialization: keep generated serializers and companions for our models.
-keepclassmembers class kotlinx.serialization.json.** { *** Companion; }
-keepclasseswithmembers class kotlinx.serialization.json.** { kotlinx.serialization.KSerializer serializer(...); }
-keep,includedescriptorclasses class dev.simhook.app.**$$serializer { *; }
-keepclassmembers class dev.simhook.app.** { *** Companion; }
-keepclasseswithmembers class dev.simhook.app.** { kotlinx.serialization.KSerializer serializer(...); }

# Readable stack traces in crash reports users send us.
-keepattributes SourceFile,LineNumberTable
-renamesourcefileattribute SourceFile
