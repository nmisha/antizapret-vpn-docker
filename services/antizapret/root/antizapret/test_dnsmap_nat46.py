from pathlib import Path
from unittest.mock import Mock
from dnslib import DNSRecord, RR, QTYPE, RCODE, AAAA, CNAME
from nat46 import HostPolicy, MappingError
from test_dnsmap import ResolverTestCase, dns_reply


class IPv6ResolverTests(ResolverTestCase):
    def resolver(self, include='example.com\n', exclude='', **kwargs):
        directory = Path(self.temp_directory.name)
        (directory / 'include-hosts-ipv6-custom.txt').write_text(include)
        (directory / 'exclude-hosts-ipv6-custom.txt').write_text(exclude)
        self.backend = Mock()
        self.backend.mappings.return_value = {}
        return self.make_resolver(nat46_backend=self.backend,
                                  ipv6_policy=HostPolicy(directory), **kwargs)

    def answer6(self, request, addresses=('2001:db8::1',)):
        answer = DNSRecord.question(str(request.q.qname), 'AAAA').reply()
        for address in addresses:
            answer.add_answer(RR(request.q.qname, QTYPE.AAAA, ttl=60, rdata=AAAA(address)))
        return answer

    def test_multiple_aaaa_and_cname_are_flattened_to_original_a(self):
        resolver = self.resolver()
        request = DNSRecord.question('example.com', 'A')
        upstream = self.answer6(request, ('2001:db8::1', '2001:db8::2'))
        for record in upstream.rr:
            record.rname = 'target.example.net'
        upstream.add_answer(RR('example.com', QTYPE.CNAME, rdata=CNAME('target.example.net')))
        resolver.query_doh = Mock(side_effect=[dns_reply(request), upstream])
        reply = resolver.resolve(request, None)
        self.assertEqual(reply.header.rcode, RCODE.NOERROR)
        self.assertEqual([str(r.rdata) for r in reply.rr], ['14.16.0.2', '14.16.0.3'])
        self.assertTrue(all(r.rtype == QTYPE.A and r.rname == request.q.qname for r in reply.rr))
        self.assertEqual(reply.header.ad, 0)
        self.assertEqual(self.backend.add.call_count, 2)
        self.assertEqual(resolver.query_doh.call_args[0][0].q.qtype, QTYPE.AAAA)
        self.assertEqual(resolver.query_doh.call_args[0][1], 'az-local')
        self.assertNotIn('2001:db8', self.iptables_runner.call_args.kwargs['input'])

    def test_no_aaaa_is_strict_servfail(self):
        resolver = self.resolver()
        request = DNSRecord.question('example.com', 'A')
        resolver.query_doh = Mock(side_effect=[dns_reply(request, ['192.0.2.1']), self.answer6(request, ())])
        self.assertEqual(resolver.resolve(request, None).header.rcode, RCODE.SERVFAIL)
        self.backend.add.assert_not_called()

    def test_unusable_aaaa_do_not_create_mappings(self):
        resolver = self.resolver()
        request = DNSRecord.question('example.com', 'A')
        resolver.query_doh = Mock(side_effect=[dns_reply(request),
            self.answer6(request, ('::', '::1', 'fe80::1', 'ff02::1'))])
        self.assertEqual(resolver.resolve(request, None).header.rcode, RCODE.SERVFAIL)
        self.backend.add.assert_not_called()

    def test_ipv6_exclude_keeps_ipv4_mapping(self):
        resolver = self.resolver(exclude='example.com\n')
        request = DNSRecord.question('www.example.com', 'A')
        resolver.query_doh = Mock(return_value=dns_reply(request, ['192.0.2.1']))
        self.assertEqual(str(resolver.resolve(request, None).rr[0].rdata), '14.16.0.2')
        self.backend.add.assert_not_called()
        self.assertEqual(resolver.query_doh.call_count, 1)

    def test_include_does_not_bypass_exit_selection(self):
        resolver = self.resolver()
        request = DNSRecord.question('example.com', 'A')
        resolver.is_blocked_asn = Mock(return_value=False)
        resolver.query_doh = Mock(side_effect=[dns_reply(request, rcode=RCODE.SERVFAIL), dns_reply(request, ['192.0.2.1'])])
        self.assertEqual(resolver.resolve(request, None).header.rcode, RCODE.SERVFAIL)
        self.backend.add.assert_not_called()
        self.assertEqual(resolver.query_doh.call_count, 2)

    def test_partial_backend_failure_rolls_back_and_quarantines_addresses(self):
        resolver = self.resolver()
        self.backend.add.side_effect = [None, MappingError('ambiguous')]
        self.backend.remove.side_effect = MappingError('cannot confirm deletion')
        available = len(resolver.unassigned_addresses)
        result = resolver.add_mappings(['2001:db8::1', '2001:db8::2'])
        self.assertEqual(result, {'2001:db8::1': None, '2001:db8::2': None})
        self.assertEqual(self.backend.remove.call_count, 2)
        self.assertEqual(len(resolver.unassigned_addresses), available - 2)
        self.assertEqual(resolver.ipmap, {})

    def test_exhaustion_does_not_flush_established_mappings(self):
        resolver = self.resolver(iprange='14.16.0.0/30')
        self.assertTrue(resolver.add_mapping('2001:db8::1'))
        self.assertFalse(resolver.add_mapping('192.0.2.1'))
        self.assertEqual(resolver.get_mapping('2001:db8::1'), '14.16.0.2')
        self.assertFalse(any('-F' in call.args[0] for call in self.iptables_runner.call_args_list))

    def test_restoration_reserves_ipv6_addresses_in_shared_pool(self):
        import ipaddress
        backend = Mock()
        backend.mappings.return_value = {ipaddress.IPv4Address('14.16.0.2'): ipaddress.IPv6Address('2001:db8::1')}
        resolver = self.make_resolver(nat46_backend=backend)
        self.assertEqual(resolver.add_mapping('192.0.2.1'), '14.16.0.3')

    def test_address_families_are_typed_and_ipv6_aliases_deduplicated(self):
        import ipaddress
        resolver = self.resolver()
        self.assertEqual(resolver.add_mapping('2001:db8::1'), '14.16.0.2')
        self.assertEqual(resolver.add_mapping('2001:0db8:0:0:0:0:0:1'), '14.16.0.2')
        self.assertEqual(resolver.add_mapping('192.0.2.1'), '14.16.0.3')
        self.assertEqual(set(resolver.ipmap), {ipaddress.IPv6Address('2001:db8::1'),
                                             ipaddress.IPv4Address('192.0.2.1')})
        self.assertEqual(self.backend.add.call_count, 1)

    def test_partial_cname_reply_is_followed_with_same_client(self):
        resolver = self.resolver()
        request = DNSRecord.question('example.com')
        alias = self.answer6(request, ())
        alias.add_answer(RR('example.com', QTYPE.CNAME, rdata=CNAME('cdn.example.net')))
        target = self.answer6(DNSRecord.question('cdn.example.net'))
        resolver.query_doh = Mock(side_effect=[dns_reply(request), alias, target])
        reply = resolver.resolve(request, None)
        self.assertEqual(reply.header.rcode, RCODE.NOERROR)
        self.assertEqual(str(reply.rr[0].rname), 'example.com.')
        query, client = resolver.query_doh.call_args.args
        self.assertEqual(str(query.q.qname), 'cdn.example.net.')
        self.assertEqual(client, 'az-local')

    def test_cname_loop_and_depth_limit_fail_without_installation(self):
        for loop in (True, False):
            with self.subTest(loop=loop):
                resolver = self.resolver()
                count = 0
                def upstream(q, client):
                    nonlocal count
                    count += 1
                    reply = q.reply()
                    target = 'example.com' if loop else 'alias{}.example.net'.format(count)
                    reply.add_answer(RR(q.q.qname, QTYPE.CNAME, rdata=CNAME(target)))
                    return reply
                resolver.query_doh = Mock(side_effect=upstream)
                # Call helper directly to avoid the unrelated initial A query.
                with self.assertRaises(ValueError):
                    resolver.resolve_ipv6_addresses('example.com', 'az-local')
                self.assertLessEqual(count, resolver.IPV6_CNAME_MAX_HOPS + 1)
                self.backend.add.assert_not_called()

    def test_denied_alias_and_unrelated_aaaa_are_not_used(self):
        resolver = self.resolver()
        request = DNSRecord.question('example.com')
        alias = self.answer6(request, ())
        alias.add_answer(RR('example.com', QTYPE.CNAME, rdata=CNAME('cdn.example.net')))
        unrelated = self.answer6(DNSRecord.question('unrelated.example.net'))
        for replies in ([alias, dns_reply(request, rcode=RCODE.SERVFAIL)], [unrelated]):
            resolver.query_doh = Mock(side_effect=replies)
            self.assertEqual(resolver.resolve_ipv6_addresses('example.com', 'az-local'), [])
        self.backend.add.assert_not_called()

    def test_checkpoint_completes_before_dns_allocation_is_published(self):
        import threading
        from concurrent.futures import ThreadPoolExecutor
        checkpoint = Path(self.temp_directory.name) / 'nat46.rules'
        resolver = self.resolver(nat46_checkpoint=checkpoint)
        entered, release = threading.Event(), threading.Event()
        def save(*args):
            entered.set()
            if not release.wait(5):
                raise RuntimeError('test checkpoint timed out')
        self.backend.save.side_effect = save
        with ThreadPoolExecutor() as pool:
            future = pool.submit(resolver.add_mapping, '2001:db8::1')
            try:
                self.assertTrue(entered.wait(2))
                self.assertFalse(future.done())
                self.assertIsNone(resolver.get_mapping('2001:db8::1'))
            finally:
                release.set()
            self.assertEqual(future.result(timeout=2), '14.16.0.2')
        self.backend.save.assert_called_once_with(checkpoint, resolver.iprange)
        # Reusing an advertised mapping does not write another checkpoint.
        resolver.add_mapping('2001:db8::1')
        self.assertEqual(self.backend.save.call_count, 1)

    def test_failed_checkpoint_returns_servfail_and_rolls_back(self):
        resolver = self.resolver(nat46_checkpoint=Path(self.temp_directory.name) / 'nat46.rules')
        self.backend.save.side_effect = OSError('disk full')
        request = DNSRecord.question('example.com')
        resolver.query_doh = Mock(side_effect=[dns_reply(request), self.answer6(request)])
        self.assertEqual(resolver.resolve(request, None).header.rcode, RCODE.SERVFAIL)
        self.backend.remove.assert_called_once_with('14.16.0.2', '2001:db8::1')
        self.assertIsNone(resolver.get_mapping('2001:db8::1'))
        self.assertNotIn('14.16.0.2', resolver.unassigned_addresses)
        self.iptables_runner.assert_called_once()

    def test_ipv4_allocation_also_waits_for_combined_checkpoint(self):
        resolver = self.resolver(nat46_checkpoint=Path(self.temp_directory.name) / 'nat46.rules')
        self.backend.save.side_effect = OSError('disk full')
        self.assertFalse(resolver.add_mapping('192.0.2.20'))
        self.assertIsNone(resolver.get_mapping('192.0.2.20'))
        self.assertNotIn('14.16.0.2', resolver.unassigned_addresses)
        self.iptables_runner.assert_called_once()
        self.backend.save.assert_called_once()

    def test_disabled_checkpoint_does_not_change_mapping_installation(self):
        resolver = self.resolver(nat46_checkpoint=None)
        self.assertTrue(resolver.add_mapping('2001:db8::1'))
        self.backend.save.assert_not_called()

    def test_policy_reload_and_invalid_edits(self):
        resolver = self.resolver(include='/^api[0-9]+\\.example\\.com$/\n')
        self.assertTrue(resolver.ipv6_policy.selected('api1.example.com.'))
        self.assertFalse(resolver.ipv6_policy.selected('other.example.com'))
        path = Path(self.temp_directory.name) / 'exclude-hosts-ipv6-custom.txt'
        path.write_text('example.com\n')
        self.assertFalse(resolver.ipv6_policy.selected('api1.example.com'))
        path.write_text('/[/\n')
        with self.assertRaises(Exception):
            resolver.ipv6_policy.selected('api1.example.com')
