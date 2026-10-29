from jobseeker.geo import distance_km, locate, locate_all, work_mode


def test_locates_town_names():
    assert locate("Antwerpen") is not None
    assert locate("Somewhere unknown") is None


def test_longest_name_wins():
    # "Louvain-la-Neuve" must not also count as "Louvain" (Leuven)
    assert len(locate_all("Louvain-la-Neuve")) == 1
    assert len(locate_all("Leuven, Louvain-la-Neuve")) == 2


def test_postcode_fallback():
    assert locate("2000") is not None


def test_distance_is_plausible():
    km = distance_km(locate("Antwerp"), locate("Leuven"))
    assert 45 < km < 75  # ~45 km straight line, times the road factor


def test_work_mode():
    assert work_mode("This is a fully remote position") == "remote"
    assert work_mode("Hybrid, 2 days at the office") == "hybrid"
    assert work_mode("At our office in Ghent") == "unknown"
