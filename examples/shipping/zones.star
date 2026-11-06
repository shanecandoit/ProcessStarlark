Process Zones:
    """Which shipping zone each destination country belongs to."""

    example: zone("US") == "domestic"
    example: zone("CA") == "north-america"
    example: zone("MX") == "north-america"
    example: zone("FR") == "international"

    def zone(country):
        """Example-only: the operations team keeps this table."""
        pass
