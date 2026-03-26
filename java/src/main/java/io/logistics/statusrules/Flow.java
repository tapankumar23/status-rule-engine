package io.logistics.statusrules;

/**
 * Represents the direction of a shipment lifecycle.
 *
 * <p>FORWARD covers the normal delivery path (booking → delivery).
 * REVERSE covers the return path (return-initiated → RTO-delivered).
 *
 * <p>The integer {@code value} is stored in the adjacency-table key and must
 * not be changed without re-generating the packed key space.
 */
public enum Flow {
    FORWARD(1),
    REVERSE(2);

    /** Wire value embedded in adjacency-table keys. */
    public final int value;

    Flow(int v) {
        this.value = v;
    }
}
