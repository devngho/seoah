plugins {
    kotlin("jvm") version "2.4.20"
    kotlin("plugin.serialization") version "2.4.20"
    id("io.insert-koin.compiler.plugin") version "1.2.1"
    application
}

group = "io.github.devngho"
version = "0.1.0"

repositories {
    mavenCentral()
}

dependencies {
    implementation("net.dv8tion:JDA:6.6.0")
    implementation("club.minnced:jda-ktx:0.15.0")
    // Discord voice is end-to-end encrypted (DAVE)
    implementation("club.minnced:jdave-api:0.1.8")
    implementation("club.minnced:jdave-native-linux-x86-64:0.1.8")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.11.0")
    implementation("com.akuleshov7:ktoml-core:0.7.1")
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.9.0")

    implementation(platform("io.insert-koin:koin-bom:4.2.0"))
    implementation("io.insert-koin:koin-core")

    implementation("io.github.oshai:kotlin-logging-jvm:8.0.4")
    implementation("org.slf4j:slf4j-simple:2.0.19")

    implementation("com.google.genai:google-genai:1.76.0")

    testImplementation(kotlin("test"))
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.11.0")
}

application {
    mainClass = "io.github.devngho.seoah.MainKt"
    applicationDefaultJvmArgs = listOf("--enable-native-access=ALL-UNNAMED") // jdave
}

kotlin {
    jvmToolchain(25)
}

tasks.test {
    useJUnitPlatform()
}

koinCompiler {
    strictSafety = true
}