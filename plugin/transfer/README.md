# transfer

## Name

*transfer* - perform (outgoing) zone transfers for other plugins.

## Description

This plugin answers zone transfers for authoritative plugins that implement `transfer.Transferer`.

*transfer* answers full zone transfer (AXFR) requests and incremental zone transfer (IXFR) requests
with AXFR fallback if the zone has changed.

When a plugin wants to notify it's secondaries it will call back into the *transfer* plugin.

## Syntax

~~~
transfer [ZONE...] {
  to HOST...
}
~~~

*  **ZONE** The zones *transfer* will answer zone transfer requests for. If left blank, the zones
    are inherited from the enclosing server block. To answer zone transfers for a given zone,
    there must be another plugin in the same server block that serves the same zone, and implements
    `transfer.Transferer`.

*  `to` **HOST...** The hosts *transfer* will transfer to. Use `*` to permit transfers to all
    hosts.

## Examples

do them
