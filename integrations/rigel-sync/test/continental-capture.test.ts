// py-continental's capture (#67): what it keeps of a response, and that it
// is offered only when asked for. Every value here is made up.
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { connectors } from '../src/connectors/index.ts';
import { scrub } from '../src/connectors/py/continental.ts';

test('a capture drops token-like fields and masks long numbers to their last four', () => {
  assert.deepEqual(scrub({
    accessToken: 'eyJ...', jwt: 'x', cuentas: [{ numeroCuenta: '142600001234', saldo: 2500, moneda: 'USD', hashCuenta: 'abc' }],
    tarjeta: { numero: '4677 9100 0000 6630', pin: '1234' }, nested: { refreshToken: 'r', movimientos: [{ monto: 1708, ref: 2145246, id: 12345678901 }] },
  }), {
    cuentas: [{ numeroCuenta: '******1234', saldo: 2500, moneda: 'USD', hashCuenta: 'abc' }],
    tarjeta: { numero: '******6630' },
    nested: { movimientos: [{ monto: 1708, ref: 2145246, id: '******8901' }] },
  });
});

test('the capture connector is offered only with RIGEL_SYNC_EXPERIMENTAL=1', () => {
  delete process.env.RIGEL_SYNC_EXPERIMENTAL;
  assert.ok(!connectors({ fake: false }).some((c) => c.id === 'py-continental'));
  process.env.RIGEL_SYNC_EXPERIMENTAL = '1';
  assert.ok(connectors({ fake: false }).some((c) => c.id === 'py-continental'));
  delete process.env.RIGEL_SYNC_EXPERIMENTAL;
});
