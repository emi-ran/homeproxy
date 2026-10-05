import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:homeproxy_agent/main.dart';

void main() {
  testWidgets('Agent form validates empty settings', (tester) async {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(
          const MethodChannel('homeproxy/agent'),
          (call) async => call.method == 'loadSettings'
              ? <String, dynamic>{}
              : 'Durduruldu',
        );
    await tester.pumpWidget(const HomeProxyApp());
    await tester.pump();
    await tester.scrollUntilVisible(
      find.text('Bağlantıyı başlat'),
      250,
      scrollable: find
          .descendant(
            of: find.byType(ListView),
            matching: find.byType(Scrollable),
          )
          .first,
    );
    await tester.tap(find.text('Bağlantıyı başlat'));
    await tester.pump();
    expect(find.text('Sunucu adresi gerekli'), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
  });
  testWidgets('Saved settings restore without starting tunnel', (tester) async {
    final calls = <String>[];
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(const MethodChannel('homeproxy/agent'), (
          call,
        ) async {
          calls.add(call.method);
          if (call.method == 'loadSettings') {
            return {
              'address': 'example.com:4433',
              'id': 'telefon',
              'token': 'test-token-not-secret',
              'fingerprint': '',
              'insecure': true,
            };
          }
          return 'Durduruldu';
        });
    await tester.pumpWidget(const HomeProxyApp());
    await tester.pump();
    expect(find.text('example.com:4433'), findsOneWidget);
    expect(calls, isNot(contains('start')));
    await tester.pumpWidget(const SizedBox());
  });
}
