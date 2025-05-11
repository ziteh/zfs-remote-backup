from abc import ABCMeta, abstractmethod


class Hasher(metaclass=ABCMeta):
    @abstractmethod
    def update(self, data: bytes) -> None:
        raise NotImplementedError()

    @abstractmethod
    def reset(self) -> None:
        raise NotImplementedError()

    @property
    @abstractmethod
    def digest(self) -> bytes:
        raise NotImplementedError()

    @property
    @abstractmethod
    def hexdigest(self) -> str:
        raise NotImplementedError()


class SumHasher(Hasher):
    _sum: int

    def __init__(self) -> None:
        self.reset()

    def update(self, data: bytes) -> None:
        self._sum = (self._sum + sum(data)) % 2**32

    def reset(self) -> None:
        self._sum = 0

    @property
    def digest(self) -> bytes:
        return self._sum.to_bytes(4, "big", signed=False)

    @property
    def hexdigest(self) -> str:
        return self.digest.hex()
