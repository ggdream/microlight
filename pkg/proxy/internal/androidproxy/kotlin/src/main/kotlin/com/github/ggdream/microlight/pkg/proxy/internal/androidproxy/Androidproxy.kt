package com.github.ggdream.microlight.pkg.proxy.internal.androidproxy

import android.content.Context
import android.net.ConnectivityManager
import android.net.Uri
import dev.mygo.Native
import java.io.IOException
import java.net.InetSocketAddress
import java.net.Proxy
import java.net.ProxySelector
import java.net.URI
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.runInterruptible

fun makeResolver(): Resolver = AndroidResolver()

private class AndroidResolver : Resolver {
    override suspend fun routes(target: String): List<String> = runInterruptible(Dispatchers.IO) {
        val manager = Native.context.getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
        val config = manager.defaultProxy ?: return@runInterruptible listOf("direct://")
        // Android downloads PAC and publishes a local proxy port when ready.
        // Do not silently bypass an automatic proxy while it is starting.
        if (config.pacFileUrl != Uri.EMPTY && config.port <= 0) {
            throw IOException("Android proxy auto-configuration is not ready")
        }
        val selector = ProxySelector.getDefault()
            ?: throw IOException("Android system proxy selector is unavailable")
        val routes = selector.select(URI(target))
        if (routes.isEmpty()) throw IOException("Android returned no proxy routes")
        routes.map { proxy ->
            when (proxy.type()) {
                Proxy.Type.DIRECT -> "direct://"
                Proxy.Type.HTTP, Proxy.Type.SOCKS -> {
                    val address = proxy.address() as? InetSocketAddress
                        ?: throw IOException("Android returned an invalid proxy address")
                    val host = address.hostString.let { if (it.contains(':')) "[$it]" else it }
                    val scheme = if (proxy.type() == Proxy.Type.SOCKS) "socks5" else "http"
                    "$scheme://$host:${address.port}"
                }
            }
        }
    }
}
