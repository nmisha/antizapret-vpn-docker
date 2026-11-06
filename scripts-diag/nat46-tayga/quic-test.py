"""Actual QUIC stream echo; invoke inside test server/client containers."""
import asyncio
import ssl
import sys
from aioquic.asyncio import connect, serve
from aioquic.quic.configuration import QuicConfiguration


async def echo(reader, writer):
    payload = await reader.read()
    writer.write(payload)
    writer.write_eof()
    await writer.drain()


async def main():
    if sys.argv[1] == 'server':
        configuration = QuicConfiguration(is_client=False, alpn_protocols=['nat46'])
        configuration.load_cert_chain('/tmp/quic.crt', '/tmp/quic.key')
        await serve('fd46:48::20', 443, configuration=configuration,
                    stream_handler=lambda r, w: asyncio.create_task(echo(r, w)))
        await asyncio.Event().wait()
    else:
        configuration = QuicConfiguration(is_client=True, alpn_protocols=['nat46'])
        configuration.verify_mode = ssl.CERT_NONE  # isolated, self-signed test server
        async with connect('14.18.0.2', 443, configuration=configuration,
                           local_port=40000) as protocol:
            reader, writer = await protocol.create_stream()
            payload = b'nat46-quic-stream-' * 4096
            writer.write(payload)
            writer.write_eof()
            received = await asyncio.wait_for(reader.readexactly(len(payload)), 15)
            assert received == payload
            print('QUIC/443 stream echo OK:', len(received), 'bytes')


asyncio.run(main())
