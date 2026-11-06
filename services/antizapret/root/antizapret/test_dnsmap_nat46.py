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
