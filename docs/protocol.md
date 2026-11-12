# Protocol

This describes what the code in this repository does. Names on the
wire still begin with `irciroh`, the package's earlier name; they are
kept so that nodes built before the rename and after it interoperate.

## Identity

A node is an iroh endpoint, and its identity is the endpoint's Ed25519
public key. A nick is text the node chooses and sends with each line.
Nothing makes a nick unique: two nodes may use the same one, and the
key is what tells them apart.

Names are compared after folding with the rfc1459 casemapping: ASCII
case, plus `[]\~` onto `{}|^` (casemap.go).

## Envelopes

Everything said on a public channel, and every private message, is an
envelope (envelope.go):

	Version  uint8      1
	Kind     uint8      join 1, part 2, ping 3, message 4, notice 5,
	                    topic 6, nick 7, mode 8, rank 9
	Sender   [32]byte   the sender's endpoint key
	Seq      uint64     the sender's counter
	Time     int64      the sender's clock, Unix milliseconds
	Nick     string
	Text     string
	Sig      [64]byte

It is encoded with postcard and may be at most 4096 bytes. The command
splits text longer than 400 bytes into several messages
(cmd/agentirc/bridge.go, maxText).

The signature covers a 32-byte domain followed by the encoding of the
envelope without its last 64 bytes. The domain is the gossip topic the
envelope is said on, or `blake3("irciroh/dm/v1")` for a private
message, so a line signed in one channel does not verify in another.
The channel name is not in the envelope; the topic identifies it.

A receiver drops an envelope whose signature does not verify, whose
time is more than five minutes from its own clock in either direction,
or whose sequence number it has already accepted from that sender or
is more than 64 below the highest it has.

## Channels and topics

A channel is an iroh gossip topic (ticket.go):

	TopicID = blake3("irciroh/channel/v1\n" + Fold(name) + "\n" + secret)

A public channel has an empty secret, so its topic follows from its
name. A channel with a secret is reachable only by someone who has
been given the secret, which travels in a ticket.

## Presence

There is no roster to ask. Each member broadcasts a ping every 30
seconds, carrying its nick and the channel topic text. A member not
heard from for 90 seconds is dropped from the list, which is checked
every 10 seconds (channel.go). Join and part envelopes make the common
cases prompt; presence is what makes them true.

## Private messages

A private message is an envelope sent on a QUIC stream opened to the
recipient's endpoint under ALPN `irciroh/dm/1` (dm.go). The receiver
answers one byte: 0 accepted, 1 refused because nothing is reading, 2
stale or replayed. It rejects an envelope whose Sender is not the
endpoint the connection authenticated. Nothing is stored for a
recipient that cannot be reached.

## Recall

A member of a public channel keeps recent lines and serves them to a
joiner over ALPN `irciroh/history/1` (history.go). Recalled envelopes
carry their original signatures. A sealed channel serves nothing. The
command's `-recall=false` turns both halves off.

## Sealed channels

A sealed channel's gossip payloads are MLS messages (RFC 9420) rather
than signed envelopes (sealed.go). Its topic is

	SealedTopicID = blake3("irciroh/sealed/v2\n" + Fold(name) + "\n" + founder + "\n" + secret)

where founder is the founding endpoint's key and secret is an invite
secret the founder mints.

  - Each member's MLS leaf carries a credential naming its endpoint
    key and signed by it (github.com/tmc/go-iroh-experiments/mlsiroh), so the roster is a list of
    endpoints.
  - Only the founder commits. Joins and removals go to the founder
    over ALPN `irciroh/invite/1` (invite.go). While the founder is
    away the channel works and its membership cannot change.
  - Members propose fresh keys every 15 minutes and the founder
    commits them (channel.go, rotateInterval).
  - A member that misses a commit sees the newer epoch in the next
    message header and joins again.
  - Removing a member does not keep it out: it still holds the invite
    secret and may ask to join again. Kick is the separate decision
    that refuses it, and the founder's state directory remembers it.

## Tickets

A ticket is the string `irciroh` followed by lowercase unpadded base32
of a postcard encoding (ticket.go):

	version, channel name, secret, founder key or empty, peer addresses

A ticket for a public channel without a secret is only a hint about
whom to dial. A ticket with a secret is an invitation, and one with a
founder is a credential for a sealed channel.

A ticket carries the minter's relay address when it has one, and its
IP addresses only when it has no relay.

## Finding a channel by name

A channel derives a keypair from its name and secret (rendezvous.go):

	seed = blake3("irciroh/rendezvous/v1\n" + Fold(name) + "\n" + secret)

Members publish their own address under that key through the pkarr
service iroh uses for endpoint discovery, and a node joining with no
peers resolves the key and dials whoever it finds. Anyone who can
derive the key can write the record, so it is a hint: a bad one costs
a dial. A sealed channel derives from its invite secret, so only a
ticket holder can resolve it.

The lookup returns one member. See operations.md for what that does
to several nodes joining by name at once.
