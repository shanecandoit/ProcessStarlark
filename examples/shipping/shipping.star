Process Shipping:
    """Quote shipping in cents from a destination country and a weight in grams."""

    example: quote("US", 500) == 599
    example: quote("CA", 1200) == 2199
    example: quote("FR", 2500) == 3999

    def quote(country, grams):
        """A base fee per zone, plus a fee for each started 500 g."""
        base = {"domestic": 499, "north-america": 1299, "international": 1999}
        per_step = {"domestic": 100, "north-america": 300, "international": 400}
        zone = Zones.zone(country)
        steps = (grams + 499) // 500
        cents = base[zone] + per_step[zone] * steps
        return cents
