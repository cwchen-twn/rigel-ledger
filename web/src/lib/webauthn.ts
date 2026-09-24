// Passkeys (WebAuthn) in the browser. The server sends the standard JSON
// form of the options (binary fields as base64url) and reads the standard
// JSON form of the credential back. Newer browsers convert natively
// (parse*OptionsFromJSON, toJSON); the fallbacks cover the rest.

const b64urlToBuf = (s: string): ArrayBuffer => {
  const pad = '='.repeat((4 - (s.length % 4)) % 4);
  const bin = atob((s + pad).replace(/-/g, '+').replace(/_/g, '/'));
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out.buffer;
};

const bufToB64url = (b: ArrayBuffer | null | undefined): string | undefined => {
  if (!b) return undefined;
  let bin = '';
  for (const byte of new Uint8Array(b)) bin += String.fromCharCode(byte);
  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
};

type Json = Record<string, any>; // eslint-disable-line @typescript-eslint/no-explicit-any

const PKC = () => window.PublicKeyCredential as unknown as {
  parseCreationOptionsFromJSON?: (o: Json) => PublicKeyCredentialCreationOptions;
  parseRequestOptionsFromJSON?: (o: Json) => PublicKeyCredentialRequestOptions;
};

export function passkeysSupported(): boolean {
  return typeof window.PublicKeyCredential === 'function' && !!navigator.credentials;
}

function creationOptions(o: Json): PublicKeyCredentialCreationOptions {
  const native = PKC().parseCreationOptionsFromJSON;
  if (native) return native(o);
  return {
    ...o,
    challenge: b64urlToBuf(o.challenge),
    user: { ...o.user, id: b64urlToBuf(o.user.id) },
    excludeCredentials: (o.excludeCredentials ?? []).map((c: Json) => ({ ...c, id: b64urlToBuf(c.id) })),
  } as PublicKeyCredentialCreationOptions;
}

function requestOptions(o: Json): PublicKeyCredentialRequestOptions {
  const native = PKC().parseRequestOptionsFromJSON;
  if (native) return native(o);
  return {
    ...o,
    challenge: b64urlToBuf(o.challenge),
    allowCredentials: (o.allowCredentials ?? []).map((c: Json) => ({ ...c, id: b64urlToBuf(c.id) })),
  } as PublicKeyCredentialRequestOptions;
}

function credentialJSON(c: PublicKeyCredential): Json {
  const withJSON = c as PublicKeyCredential & { toJSON?: () => Json };
  if (typeof withJSON.toJSON === 'function') return withJSON.toJSON();
  const response: Json = { clientDataJSON: bufToB64url(c.response.clientDataJSON) };
  if ('attestationObject' in c.response) {
    const r = c.response as AuthenticatorAttestationResponse;
    response.attestationObject = bufToB64url(r.attestationObject);
    response.transports = r.getTransports?.() ?? [];
  } else {
    const r = c.response as AuthenticatorAssertionResponse;
    response.authenticatorData = bufToB64url(r.authenticatorData);
    response.signature = bufToB64url(r.signature);
    response.userHandle = bufToB64url(r.userHandle);
  }
  return {
    id: c.id, rawId: bufToB64url(c.rawId), type: c.type, response,
    authenticatorAttachment: c.authenticatorAttachment ?? undefined,
    clientExtensionResults: c.getClientExtensionResults(),
  };
}

/** Create a passkey from the server's creation options ({publicKey: ...}). */
export async function createPasskey(options: Json): Promise<Json> {
  const cred = (await navigator.credentials.create({ publicKey: creationOptions(options.publicKey ?? options) })) as PublicKeyCredential | null;
  if (!cred) throw new Error('cancelled');
  return credentialJSON(cred);
}

/** Sign in with a passkey from the server's request options ({publicKey: ...}). */
export async function getPasskey(options: Json): Promise<Json> {
  const cred = (await navigator.credentials.get({ publicKey: requestOptions(options.publicKey ?? options) })) as PublicKeyCredential | null;
  if (!cred) throw new Error('cancelled');
  return credentialJSON(cred);
}

/** The browser refused or the user cancelled: not an error worth a red toast. */
export function isCancelled(e: unknown): boolean {
  return e instanceof DOMException && (e.name === 'NotAllowedError' || e.name === 'AbortError');
}
